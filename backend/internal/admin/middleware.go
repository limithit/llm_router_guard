// middleware.go 管理 API 鉴权中间件（JWT）+ 会话吊销/受限 scope 门禁（SEC-02/09/13）。
package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/auth"
	"llmrouter/internal/model"
)

// 受限会话允许的路由（FullPath 模式）。pw：仅改密；mfa：仅绑定 MFA。
var (
	pwScopePaths = map[string]bool{
		"/api/admin/v1/auth/me": true, "/api/admin/v1/auth/password": true, "/api/admin/v1/auth/logout": true,
	}
	mfaScopePaths = map[string]bool{
		"/api/admin/v1/auth/me": true, "/api/admin/v1/auth/logout": true,
		"/api/admin/v1/account/mfa/setup": true, "/api/admin/v1/account/mfa/enable": true,
		"/api/admin/v1/account/mfa/status": true, "/api/admin/v1/account/mfa/disable": true,
	}
)

// AuthMiddleware 校验 Authorization: Bearer <JWT>，写入 operator/userID。
// 逐请求核验：用户仍存在、会话未被吊销（改密/登出/解绑后置失效时刻）、
// 强制改密与全员 MFA 在服务端硬性生效（受限 scope 只放行引导路由白名单）。
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			s.fail(c, http.StatusUnauthorized, 40101, "未登录或 Token 缺失")
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		claims, err := auth.Parse(s.secret, token)
		if err != nil {
			s.fail(c, http.StatusUnauthorized, 40101, "Token 无效或已过期，请重新登录")
			c.Abort()
			return
		}
		// SEC-13：吊销检查（登出/改密/解绑 MFA 后旧 Token 即刻失效）
		var u struct {
			ID                    uint
			MFAEnabled            bool
			MustChangePassword    bool
			SessionsInvalidBefore *time.Time
		}
		if err := s.db.Model(&model.AdminUser{}).
			Select("id", "mfa_enabled", "must_change_password", "sessions_invalid_before").
			First(&u, claims.UserID).Error; err != nil {
			s.fail(c, http.StatusUnauthorized, 40101, "会话已失效，请重新登录")
			c.Abort()
			return
		}
		if u.SessionsInvalidBefore != nil && claims.IssuedAt != nil &&
			claims.IssuedAt.Before(*u.SessionsInvalidBefore) {
			s.fail(c, http.StatusUnauthorized, 40101, "会话已被注销，请重新登录")
			c.Abort()
			return
		}
		switch claims.Scope {
		case "pw":
			if !u.MustChangePassword {
				s.fail(c, http.StatusUnauthorized, 40101, "会话状态已变化，请重新登录")
				c.Abort()
				return
			}
			if !pwScopePaths[c.FullPath()] {
				s.fail(c, http.StatusForbidden, 40302, "请先修改初始密码")
				c.Abort()
				return
			}
		case "mfa":
			sec := s.securitySettings()
			if !sec.MFAEnabled || !sec.MFARequiredAll || u.MFAEnabled {
				s.fail(c, http.StatusUnauthorized, 40101, "会话状态已变化，请重新登录")
				c.Abort()
				return
			}
			if !mfaScopePaths[c.FullPath()] {
				s.fail(c, http.StatusForbidden, 40303, "请先完成 MFA 绑定")
				c.Abort()
				return
			}
		default:
			// SEC-02：强制改密；SEC-09：全员 MFA —— 服务端硬性门禁（受限令牌由登录时签发）
			if u.MustChangePassword {
				s.fail(c, http.StatusForbidden, 40302, "请先修改初始密码")
				c.Abort()
				return
			}
			if !u.MFAEnabled {
				sec := s.securitySettings()
				if sec.MFAEnabled && sec.MFARequiredAll {
					s.fail(c, http.StatusForbidden, 40303, "请先完成 MFA 绑定")
					c.Abort()
					return
				}
			}
		}
		c.Set("operator", claims.Username)
		c.Set("userID", claims.UserID)
		c.Next()
	}
}
