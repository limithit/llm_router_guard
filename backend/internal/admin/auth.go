// auth.go 绠＄悊鍚庡彴鐧诲綍 / 涓汉淇℃伅 / 淇敼瀵嗙爜 / MFA 鑷姪缁戝畾锛圧EQ-020~022锛夈€?package admin

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"llmrouter/internal/auth"
	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

// pendingMFA 瀛樺偍 MFA 浜屾鐧诲綍鐨勪复鏃剁エ鎹紙5 鍒嗛挓鏈夋晥锛夈€?type pendingMFA struct {
	userID uint
	expiry time.Time
}

// pendingSetup 瀛樺偍鑷姪缁戝畾娴佺▼涓敓鎴愮殑瀵嗛挜锛? 鍒嗛挓鏈夋晥锛夈€?type pendingSetup struct {
	secret string
	expiry time.Time
}

type mfaState struct {
	login sync.Map // mfa_token(hex) -> pendingMFA
	setup sync.Map // userID -> pendingSetup
}

func (s *Server) mfa() *mfaState { return &s.mfaStateInstance }

// ---- 鐧诲綍 ----

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		TotpCode     string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
		MFAToken     string `json:"mfa_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, 40001, "璇锋眰鍙傛暟閿欒")
		return
	}
	var u model.AdminUser
	if err := s.db.Where("username = ?", req.Username).First(&u).Error; err != nil {
		s.fail(c, http.StatusUnauthorized, 40102, "鐢ㄦ埛鍚嶆垨瀵嗙爜閿欒")
		return
	}
	if u.IsLocked() {
		s.fail(c, http.StatusForbidden, 40301, "璐︽埛宸茶閿佸畾锛岃绋嶅悗鍐嶈瘯鎴栬仈绯荤鐞嗗憳瑙ｉ攣")
		return
	}

	sec := s.securitySettings()

	// MFA 浜屾楠岃瘉锛堝叏灞€寮€鍏冲紑鍚?涓?鐢ㄦ埛宸茬粦瀹氾級
	if req.MFAToken != "" {
		v, ok := s.mfa().login.LoadAndDelete(req.MFAToken)
		if !ok || !v.(pendingMFA).expiry.After(time.Now()) {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 绁ㄦ嵁宸插け鏁堬紝璇烽噸鏂扮櫥褰?)
			return
		}
		if v.(pendingMFA).userID != u.ID {
			s.fail(c, http.StatusUnauthorized, 40103, "MFA 绁ㄦ嵁涓庣敤鎴蜂笉鍖归厤")
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
			s.fail(c, http.StatusUnauthorized, 40103, "鍔ㄦ€侀獙璇佺爜鎴栨仮澶嶇爜閿欒")
			return
		}
		s.issueLogin(c, &u, sec)
		return
	}

	// 涓€绾э細瀵嗙爜鏍￠獙
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		s.onMFAFail(&u)
		s.fail(c, http.StatusUnauthorized, 40102, "鐢ㄦ埛鍚嶆垨瀵嗙爜閿欒")
		return
	}
	u.FailedLogins = 0
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		Updates(map[string]any{"failed_logins": 0})

	// 浜岀骇锛歁FA锛堝凡缁戝畾涓斿叏灞€寮€鍚級
	if sec.MFAEnabled && u.MFAEnabled {
		if req.TotpCode == "" && req.RecoveryCode == "" {
			token := crypto.RandomHex(16)
			s.mfa().login.Store(token, pendingMFA{userID: u.ID, expiry: time.Now().Add(5 * time.Minute)})
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
			s.fail(c, http.StatusUnauthorized, 40103, "鍔ㄦ€侀獙璇佺爜鎴栨仮澶嶇爜閿欒")
			return
		}
	}

	s.issueLogin(c, &u, sec)
}

// onMFAFail 杩炵画 5 娆″け璐ラ攣瀹氳处鎴凤紙REQ-022 鈶★級銆?func (s *Server) onMFAFail(u *model.AdminUser) {
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
		s.fail(c, http.StatusInternalServerError, 50001, "绛惧彂 Token 澶辫触")
		return
	}
	now := time.Now()
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Update("last_login_at", &now)
	s.recordOp(c, "login", "auth", u.Username, nil, nil)
	needBind := sec.MFAEnabled && sec.MFARequiredAll && !u.MFAEnabled
	s.ok(c, gin.H{
		"token": token,
		"user":  gin.H{"id": u.ID, "username": u.Username, "mfa_enabled": u.MFAEnabled, "last_login_at": now},
		"need_bind_mfa": needBind,
	})
}

func (s *Server) me(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "鐢ㄦ埛涓嶅瓨鍦?)
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
	var req struct{ Old, New string }
	req.New = ""
	if err := c.ShouldBindJSON(&req); err != nil || req.New == "" {
		s.fail(c, http.StatusBadRequest, 40001, "璇锋眰鍙傛暟閿欒")
		return
	}
	if len(req.New) < 8 {
		s.fail(c, http.StatusBadRequest, 40001, "鏂板瘑鐮佽嚦灏?8 浣?)
		return
	}
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "鐢ㄦ埛涓嶅瓨鍦?)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Old)); err != nil {
		s.fail(c, http.StatusUnauthorized, 40102, "鍘熷瘑鐮侀敊璇?)
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Update("password_hash", string(hash))
	s.recordOp(c, "update", "auth", "password", nil, nil)
	s.ok(c, nil)
}

// ---- 璐︽埛 MFA 鑷姪缁戝畾 (REQ-021) ----

func (s *Server) mfaStatus(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "鐢ㄦ埛涓嶅瓨鍦?)
		return
	}
	s.ok(c, gin.H{"enabled": u.MFAEnabled, "bound_at": u.MFABoundAt,
		"recovery_codes_left": countRecoveryLeft(u.RecoveryCodesJSON)})
}

func (s *Server) mfaSetup(c *gin.Context) {
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "鐢ㄦ埛涓嶅瓨鍦?)
		return
	}
	if u.MFAEnabled {
		s.fail(c, http.StatusConflict, 40901, "宸茬粦瀹?MFA锛屾棤闇€閲嶅缁戝畾")
		return
	}
	secret, url, err := newTOTPSecret(u.Username)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "鐢熸垚 TOTP 瀵嗛挜澶辫触")
		return
	}
	qr, err := qrPngBase64(url)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, 50001, "鐢熸垚浜岀淮鐮佸け璐?)
		return
	}
	s.mfa().setup.Store(uid, pendingSetup{secret: secret, expiry: time.Now().Add(5 * time.Minute)})
	s.ok(c, gin.H{"secret": secret, "otpauth_url": url, "qr_png_base64": qr})
}

func (s *Server) mfaEnable(c *gin.Context) {
	var req struct{ Code string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		s.fail(c, http.StatusBadRequest, 40001, "璇疯緭鍏ュ姩鎬侀獙璇佺爜")
		return
	}
	uid := userIDOf(c)
	v, ok := s.mfa().setup.LoadAndDelete(uid)
	if !ok || !v.(pendingSetup).expiry.After(time.Now()) {
		s.fail(c, http.StatusBadRequest, 40001, "缁戝畾娴佺▼宸茶繃鏈燂紝璇烽噸鏂板紑濮?)
		return
	}
	secret := v.(pendingSetup).secret
	if !validateTOTP(req.Code, secret) {
		s.fail(c, http.StatusUnauthorized, 40103, "鍔ㄦ€侀獙璇佺爜閿欒")
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
		s.fail(c, http.StatusBadRequest, 40001, "璇疯緭鍏ュ姩鎬侀獙璇佺爜鎴栨仮澶嶇爜")
		return
	}
	uid := userIDOf(c)
	var u model.AdminUser
	if err := s.db.First(&u, uid).Error; err != nil {
		s.fail(c, http.StatusNotFound, 40401, "鐢ㄦ埛涓嶅瓨鍦?)
		return
	}
	if !u.MFAEnabled {
		s.fail(c, http.StatusConflict, 40901, "灏氭湭缁戝畾 MFA")
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
		s.fail(c, http.StatusUnauthorized, 40103, "鍔ㄦ€侀獙璇佺爜鎴栨仮澶嶇爜閿欒")
		return
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", uid).Updates(map[string]any{
		"mfa_secret": "", "mfa_enabled": false, "mfa_bound_at": nil, "recovery_codes_json": ""})
	s.recordOp(c, "mfa_unbind", "security", operatorOf(c), nil, nil)
	s.ok(c, nil)
}
