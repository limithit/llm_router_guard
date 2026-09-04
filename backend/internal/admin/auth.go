// auth.go 管理后台登录 / 个人信息 / 修改密码 / MFA 自助绑定（REQ-020~022）。
package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"llmrouter/internal/auth"
	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

func (s *Server) mfa() mfaStore { return s.mfaStateInstance }

// ---- 登录 ----

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		TotpCode     string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
		MFAToken     string `json:"mfa_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	var u model.AdminUser
	if err := s.db.Where("username = ?", req.Username).First(&u).Error; err != nil {
		s.fail(c, http.StatusUnauthorized, 40102, "用户名或密码错误")
		return
	}
	if u.IsLocked() {
		s.fail(c, http.StatusForbidden, 40301, "账户已被锁定，请稍后再试或联系管理员解锁")
		return
	}

	sec := s.securitySettings()

	// MFA 二步验证（全局开关开启 且 用户已绑定）
	if req.MFAToken != "" {
		uid, ok := s.mfa().PopLoginTicket(req.MFAToken)
		if !ok {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 票据已失效，请重新登录")
			return
		}
		if uid != u.ID {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 票据与用户不匹配")
			return
		}
		ok2 := false
		if req.TotpCode != "" && validateTOTP(req.TotpCode, u.MFASecret) {
			ok2 = true
		} else if req.RecoveryCode != "" {
			if used, next := consumeRecoveryCode(req.RecoveryCode, u.RecoveryCodesJSON); used {
				ok2 = true
				s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
					Update("recovery_codes_json", next)
			}
		}
		if !ok2 {
			s.onMFAFail(&u)
			s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
			return
		}
		s.issueLogin(c, &u, sec)
		return
	}

	// 一级：密码校验
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		s.onMFAFail(&u)
		s.fail(c, http.StatusUnauthorized, 40102, "用户名或密码错误")
		return
	}
	u.FailedLogins = 0
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		Update("failed_logins", 0)

	// 二级：MFA（已绑定且全局开启）
	if sec.MFAEnabled && u.MFAEnabled {
		if req.TotpCode == "" && req.RecoveryCode == "" {
			token := crypto.RandomHex(16)
			s.mfa().SaveLoginTicket(token, u.ID)
			s.ok(c, gin.H{"mfa_required": true, "mfa_token": token})
			return
		}
		ok2 := false
		if req.TotpCode != "" && validateTOTP(req.TotpCode, u.MFASecret) {
			ok2 = true
		} else if req.RecoveryCode != "" {
			if used, next := consumeRecoveryCode(req.RecoveryCode, u.RecoveryCodesJSON); used {
				ok2 = true
				s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
					Update("recovery_codes_json", next)
			}
		}
		if !ok2 {
			s.onMFAFail(&u)
			s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
			return
		}
	}

	s.issueLogin(c, &u, sec)
}

// onMFAFail 连续 5 次失败锁定账户（REQ-022 ②）。
func (s *Server) onMFAFail(u *model.AdminUser) {
	u.FailedLogins++
	updates := map[string]any{"failed_logins": u.FailedLogins}
	if u.FailedLogins >= 5 {
		loc := time.Now().Add(15 * time.Minute)
		updates["locked_until"] = loc
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Updates(updates)
}

func (s *Server) issueLogin(c *gin.Context, u *model.AdminUser, sec settings.Security) {
	token, err := auth.Issue(s.secret, u.ID, u.Username)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "签发 Token 失败")
		return
	}
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Update("last_login_at", &now)
	s.recordOp(c, "login", "auth", u.Username, nil, nil)
	needBind := sec.MFAEnabled && sec.MFARequiredAll && !u.MFAEnabled
	s.ok(c, gin.H{
		"token":         token,
		"user":          gin.H{"id": u.ID, "username": u.Username, "mfa_enabled": u.MFAEnabled, "last_login_at": now},
		"need_bind_mfa": needBind,
	})
}

func (s *Server) me(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	s.ok(c, gin.H{"id": u.ID, "username": u.Username, "mfa_enabled": u.MFAEnabled,
		"created_at": u.CreatedAt, "last_login_at": u.LastLoginAt})
}

func (s *Server) logout(c *gin.Context) {
	s.recordOp(c, "login", "auth", "logout", nil, nil)
	s.ok(c, nil)
}

func (s *Server) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.NewPassword == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请求参数错误")
		return
	}
	if len(req.NewPassword) < 8 {
		s.fail(c, http.StatusBadRequest, 40001, "新密码至少 8 位")
		return
	}
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		s.fail(c, http.StatusUnauthorized, 40102, "原密码错误")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Update("password_hash", string(hash))
	s.recordOp(c, "update", "auth", "password", nil, nil)
	s.ok(c, nil)
}

// ---- 账户 MFA 自助绑定 (REQ-021) ----

func (s *Server) mfaStatus(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	s.ok(c, gin.H{"enabled": u.MFAEnabled, "bound_at": u.MFABoundAt,
		"recovery_codes_left": countRecoveryLeft(u.RecoveryCodesJSON)})
}

func (s *Server) mfaSetup(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	if u.MFAEnabled {
		s.fail(c, http.StatusConflict, 40901, "已绑定 MFA，无需重复绑定")
		return
	}
	secret, url, err := newTOTPSecret(u.Username)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "生成 TOTP 密钥失败")
		return
	}
	qr, err := qrPngBase64(url)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "生成二维码失败")
		return
	}
	s.mfa().SaveSetupSecret(uid, secret)
	s.ok(c, gin.H{"secret": secret, "otpauth_url": url, "qr_png_base64": qr})
}

func (s *Server) mfaEnable(c *gin.Context) {
	var req struct{ Code string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入动态验证码")
		return
	}
	uid := userIDOf(c)
	secret, ok := s.mfa().PopSetupSecret(uid)
	if !ok {
		s.fail(c, http.StatusBadRequest, 40001, "绑定流程已过期，请重新开始")
		return
	}
	if !validateTOTP(req.Code, secret) {
		s.fail(c, http.StatusUnauthorized, 40103, "动态验证码错误")
		return
	}
	sec := s.securitySettings()
	plain, hashed := generateRecoveryCodes(sec.RecoveryCodeCount)
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).Updates(map[string]any{
		"mfa_secret": secret, "mfa_enabled": true, "mfa_bound_at": &now,
		"recovery_codes_json": marshalRecoveryHashed(hashed),
	})
	s.recordOp(c, "mfa_bind", "security", operatorOf(c), nil, nil)
	s.ok(c, gin.H{"recovery_codes": plain})
}

func (s *Server) mfaDisable(c *gin.Context) {
	var req struct{ Code string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		s.fail(c, http.StatusBadRequest, 40001, "请输入动态验证码或恢复码")
		return
	}
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	if !u.MFAEnabled {
		s.fail(c, http.StatusConflict, 40901, "尚未绑定 MFA")
		return
	}
	ok := false
	if validateTOTP(req.Code, u.MFASecret) {
		ok = true
	} else if used, next := consumeRecoveryCode(req.Code, u.RecoveryCodesJSON); used {
		ok = true
		s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Update("recovery_codes_json", next)
	}
	if !ok {
		s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
		return
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).Updates(map[string]any{
		"mfa_secret": "", "mfa_enabled": false, "mfa_bound_at": nil, "recovery_codes_json": ""})
	s.recordOp(c, "mfa_unbind", "security", operatorOf(c), nil, nil)
	s.ok(c, nil)
}
