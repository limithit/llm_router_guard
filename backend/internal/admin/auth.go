// auth.go 管理后台登录 / 个人信息 / 修改密码 / MFA 自助绑定（REQ-020~022）。
// 安全加固（SEC-02/08/09/13、M-03/M-07/M-09/M-10）：
//   - 用户名不存在与密码错误返回同一消息、同一耗时路径（dummy bcrypt），锁定状态仅在
//     口令正确后披露（防枚举）；
//   - /auth/login per-IP 限速（loginLimit），失败计数原子自增（并发不可绕过 5 次锁定）；
//   - 登出/改密/被解绑 MFA 均吊销该用户全部在发 JWT（sessions_invalid_before）；
//   - 强制改密与「全员必须 MFA」由服务端硬性生效：签发受限 scope 会话，
//     中间件按路由白名单门控（此前只是响应里的建议位）。
package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"llmrouter/internal/auth"
	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

func (s *Server) mfa() mfaStore { return s.mfaStateInstance }

// mfaRequiredFor 是否需要 MFA：全局「强制所有用户」或该用户被单独要求（取或）。
// 注意：这里只看 MFARequiredAll —— 「强制所有用户」本身即蕴含启用 MFA，
// 此前要求 MFAEnabled && MFARequiredAll，导致只开「强制」不开主开关时静默失效。
func mfaRequiredFor(requiredAll, userRequired bool) bool {
	return requiredAll || userRequired
}

// dummyBcryptHash 用于「用户名不存在」路径的等时 bcrypt 比对（M-10：消除计时侧信道）。
var dummyBcryptHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("llm-router-guard:timing-equalizer"), bcrypt.DefaultCost)
	if err != nil {
		return []byte("$2a$10$0000000000000000000000000000000000000000000000000000")
	}
	return h
}()

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
	// SEC-08/M-03：per-IP 固定窗口限速（同时封顶匿名 bcrypt CPU 放大）
	ip := clientIP(c)
	now := time.Now()
	if !s.loginLimiter.allow(ip, now) {
		c.Header("Retry-After", itoa(s.loginLimiter.retryAfter(ip, now)))
		s.fail(c, http.StatusTooManyRequests, 42901, "尝试过于频繁，请稍后再试")
		return
	}

	var u model.AdminUser
	if err := s.db.Where("username = ?", req.Username).First(&u).Error; err != nil {
		// 不存在的用户：跑一次 dummy bcrypt 保持与真实路径等时，返回同一消息（M-10）
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(req.Password))
		s.fail(c, http.StatusUnauthorized, 40102, "用户名或密码错误")
		return
	}

	sec := s.securitySettings()

	// MFA 二步（携带一级签发的票据）：票据一次性核销 + 二次因素校验
	if req.MFAToken != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
			s.onLoginFail(&u)
			s.fail(c, http.StatusUnauthorized, 40102, "用户名或密码错误")
			return
		}
		// Peek（不核销）：验证码输错时票据仍保留，用户可直接重试，无需重新登录。
		// 此前用 Pop 导致第一次输错验证码就删掉票据，第二次正确的反被"票据已失效"挡住。
		uid, ok := s.mfa().PeekLoginTicket(req.MFAToken)
		if !ok {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 票据已失效，请重新登录")
			return
		}
		if uid != u.ID {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 票据与用户不匹配")
			return
		}
		ok2 := false
		if req.TotpCode != "" {
			if step, okStep := s.verifyTOTP(&u, req.TotpCode); okStep {
				ok2 = true
				s.persistTOTPStep(u.ID, step)
			}
		} else if req.RecoveryCode != "" {
			ok2 = s.consumeRecoveryCAS(&u, req.RecoveryCode)
		}
		if !ok2 {
			// 验证码错误：票据不核销，用户可在 TTL（5 分钟）内继续重试
			s.onLoginFail(&u)
			s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
			return
		}
		// 二次因素通过后才真正核销票据（防重放）
		s.mfa().PopLoginTicket(req.MFAToken)
		s.finishLoginOK(&u)
		s.issueLogin(c, &u, sec)
		return
	}

	// 一级：密码校验（锁定状态不在此前披露，防匿名枚举/锁定探测 —— M-10）
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		s.onLoginFail(&u)
		s.fail(c, http.StatusUnauthorized, 40102, "用户名或密码错误")
		return
	}
	if u.IsLocked() {
		s.fail(c, http.StatusForbidden, 40301, "账户已被锁定，请稍后再试或联系管理员解锁")
		return
	}
	// 锁定已过期（口令正确才走到这里）：复位计数，避免锁连环（M-03）
	if u.LockedUntil != nil {
		s.finishLoginOK(&u)
	}

	// 二级：MFA —— 只要用户已绑定，登录就必须过二次因素（不再受全局开关影响，
	// 否则「已绑定 MFA 但全局开关被关掉」会让已绑定的账户静默退回单因素登录）。
	if u.MFAEnabled {
		if req.TotpCode == "" && req.RecoveryCode == "" {
			token := crypto.RandomHex(16)
			s.mfa().SaveLoginTicket(token, u.ID)
			s.ok(c, gin.H{"mfa_required": true, "mfa_token": token})
			return
		}
		ok2 := false
		if req.TotpCode != "" {
			if step, okStep := s.verifyTOTP(&u, req.TotpCode); okStep {
				ok2 = true
				s.persistTOTPStep(u.ID, step)
			}
		} else if req.RecoveryCode != "" {
			ok2 = s.consumeRecoveryCAS(&u, req.RecoveryCode)
		}
		if !ok2 {
			s.onLoginFail(&u)
			s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
			return
		}
	}

	// 完全认证成功才复位失败计数（若在 MFA 质询前复位，「对密码+错 MFA」可无限清零绕过锁定）
	s.finishLoginOK(&u)
	s.issueLogin(c, &u, sec)
}

// finishLoginOK 原子清零失败计数并落登录时间（锁到期同时复位计数 —— M-03）。
func (s *Server) finishLoginOK(u *model.AdminUser) {
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		Updates(map[string]any{"failed_logins": 0, "locked_until": nil, "last_login_at": &now})
}

// onLoginFail 连续 5 次失败锁定账户（REQ-022 ②）。
// SEC-08：failed_logins 用 SQL 原子自增（并发不可绕过阈值），锁定按条件更新。
// M-03：上一锁已到期 → 先清零再计（否则"过期后 1 次输错 = 再锁 15 分钟"，
// 攻击者每 16 分钟一个坏请求即可把管理员永久锁在门外）。
func (s *Server) onLoginFail(u *model.AdminUser) {
	now := time.Now()
	s.db.Model(&model.AdminUser{}).
		Where("id = ? AND locked_until IS NOT NULL AND locked_until <= ?", u.ID, now).
		UpdateColumn("failed_logins", 0)
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		UpdateColumn("failed_logins", gorm.Expr("failed_logins + 1"))
	var fresh int
	if err := s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		Select("failed_logins").Scan(&fresh).Error; err != nil {
		return
	}
	if fresh >= 5 {
		loc := now.Add(15 * time.Minute)
		s.db.Model(&model.AdminUser{}).Where("id = ? AND (locked_until IS NULL OR locked_until < ?)", u.ID, now).
			Update("locked_until", loc)
	}
}

func (s *Server) issueLogin(c *gin.Context, u *model.AdminUser, sec settings.Security) {
	// SEC-02：首启随机口令账户 —— 发仅可改密的受限会话
	if u.MustChangePassword {
		s.issueScoped(c, u, "pw", gin.H{"need_change_password": true})
		return
	}
	// SEC-09：MFA 硬性要求（全局强制 或 该用户被单独要求）—— 未绑定者只发仅可绑定的受限会话
	if !u.MFAEnabled && mfaRequiredFor(sec.MFARequiredAll, u.MFARequired) {
		s.issueScoped(c, u, "mfa", gin.H{"need_bind_mfa": true})
		return
	}
	token, err := auth.Issue(s.secret, u.ID, u.Username)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "签发 Token 失败")
		return
	}
	s.recordOp(c, "login", "auth", u.Username, nil, nil)
	s.ok(c, gin.H{
		"token": token,
		"user":  gin.H{"id": u.ID, "username": u.Username, "mfa_enabled": u.MFAEnabled, "role": u.Role},
		// 与上面的强制条件保持一致（此前只判 MFAEnabled，会出现「提示要绑定但其实没强制」）
		"need_bind_mfa": mfaRequiredFor(sec.MFARequiredAll, u.MFARequired) && !u.MFAEnabled,
	})
}

// issueScoped 签发受限 scope 会话（前端据此跳转到改密/绑定引导页）。
func (s *Server) issueScoped(c *gin.Context, u *model.AdminUser, scope string, extra gin.H) {
	token, err := auth.IssueScoped(s.secret, u.ID, u.Username, scope)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "签发 Token 失败")
		return
	}
	s.recordOp(c, "login", "auth", u.Username, nil, nil)
	body := gin.H{
		"token": token,
		"user":  gin.H{"id": u.ID, "username": u.Username, "mfa_enabled": u.MFAEnabled},
	}
	for k, v := range extra {
		body[k] = v
	}
	s.ok(c, body)
}

func (s *Server) me(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "用户不存在")
		return
	}
	sec := s.securitySettings()
	// must_bind_mfa/scope 让前端在「刷新页面」后仍能恢复受限会话守卫：
	// scope=mfa 的令牌只能访问白名单路由，前端必须把用户按在绑定页上。
	s.ok(c, gin.H{
		"id": u.ID, "username": u.Username, "role": u.Role,
		"mfa_enabled": u.MFAEnabled, "mfa_required": mfaRequiredFor(sec.MFARequiredAll, u.MFARequired),
		"must_bind_mfa":        !u.MustChangePassword && !u.MFAEnabled && mfaRequiredFor(sec.MFARequiredAll, u.MFARequired),
		"scope":                scopeOf(c),
		"must_change_password": u.MustChangePassword,
		"created_at":           u.CreatedAt, "last_login_at": u.LastLoginAt,
	})
}

// scopeOf 读取中间件写入的令牌 scope（"" 表示完整会话）。
func scopeOf(c *gin.Context) string {
	if v, ok := c.Get("scope"); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func (s *Server) logout(c *gin.Context) {
	// SEC-13：登出即吊销该用户当前全部会话（改密后重新登录的场景不受影响）。
	s.invalidateSessions(userIDOf(c))
	s.recordOp(c, "logout", "auth", operatorOf(c), nil, nil)
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
	// 长度/强度统一交给 auth.ValidatePassword（需先取到用户名，见下方）
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
	// 口令强度策略（长度/字符类别/弱口令黑名单/不得含用户名）+ bcrypt 72 字节上界
	if err := auth.ValidatePassword(req.NewPassword, u.Username); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil { // M-09：错误必须可见（此前忽略错误可写入空哈希）
		s.fail(c, http.StatusInternalServerError, 50001, "口令处理失败，请重试")
		return
	}
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Updates(map[string]any{
		"password_hash": string(hash), "must_change_password": false,
		"sessions_invalid_before": &now, // SEC-13：改密后旧会话全部作废
	})
	s.recordOp(c, "update", "auth", "password", nil, nil)
	s.ok(c, gin.H{"relogin_required": true})
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
	encSecret, err := s.encodeMFASecret(secret) // M-22：静态加密
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "密钥加密失败")
		return
	}
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).Updates(map[string]any{
		"mfa_secret": encSecret, "mfa_enabled": true, "mfa_bound_at": &now,
		"recovery_codes_json": marshalRecoveryHashed(hashed), "last_totp_step": 0,
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
	if step, okStep := s.verifyTOTP(&u, req.Code); okStep {
		ok = true
		s.persistTOTPStep(u.ID, step)
	} else if s.consumeRecoveryCAS(&u, req.Code) {
		ok = true
	}
	if !ok {
		s.fail(c, http.StatusUnauthorized, 40103, "动态验证码或恢复码错误")
		return
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).Updates(map[string]any{
		"mfa_secret": "", "mfa_enabled": false, "mfa_bound_at": nil, "recovery_codes_json": ""})
	// SEC-13：解绑自身 MFA 后旧会话作废（防止以“已开 MFA 的会话”静默降级使用）
	s.invalidateSessions(uid)
	s.recordOp(c, "mfa_unbind", "security", operatorOf(c), nil, nil)
	s.ok(c, nil)
}
