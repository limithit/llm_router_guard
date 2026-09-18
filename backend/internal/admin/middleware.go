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
// selfServicePaths 同时覆盖 pwScopePaths + mfaScopePaths —— 这些是「管理自己账户安全」
// 的自助操作（改密 / 绑定解绑 MFA / 登出 / 个人信息），viewer 角色也必须放行，
// 否则只读用户被要求强制 MFA 时会死锁（绑不了 MFA → 永远进不去后台）。
var (
	pwScopePaths = map[string]bool{
		"/api/admin/v1/auth/me": true, "/api/admin/v1/auth/password": true, "/api/admin/v1/auth/logout": true,
	}
	mfaScopePaths = map[string]bool{
		"/api/admin/v1/auth/me": true, "/api/admin/v1/auth/logout": true,
		"/api/admin/v1/account/mfa/setup": true, "/api/admin/v1/account/mfa/enable": true,
		"/api/admin/v1/account/mfa/status": true, "/api/admin/v1/account/mfa/disable": true,
	}
	selfServicePaths = func() map[string]bool {
		m := make(map[string]bool, len(pwScopePaths)+len(mfaScopePaths))
		for k := range pwScopePaths {
			m[k] = true
		}
		for k := range mfaScopePaths {
			m[k] = true
		}
		return m
	}()
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
			MFARequired           bool
			MustChangePassword    bool
			SessionsInvalidBefore *time.Time
			Role                  string
		}
		if err := s.db.Model(&model.AdminUser{}).
			Select("id", "mfa_enabled", "mfa_required", "must_change_password", "sessions_invalid_before", "role").
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
			// 要求来源可以是全局「强制所有用户」或该用户被单独要求；
			// 一旦已绑定、或要求已被撤销，该受限会话立即失效（需重新登录）。
			if u.MFAEnabled || !mfaRequiredFor(sec.MFARequiredAll, u.MFARequired) {
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
			// SEC-02：强制改密；SEC-09：MFA 要求 —— 服务端硬性门禁（受限令牌由登录时签发）
			if u.MustChangePassword {
				s.fail(c, http.StatusForbidden, 40302, "请先修改初始密码")
				c.Abort()
				return
			}
			if !u.MFAEnabled {
				sec := s.securitySettings()
				if mfaRequiredFor(sec.MFARequiredAll, u.MFARequired) {
					s.fail(c, http.StatusForbidden, 40303, "请先完成 MFA 绑定")
					c.Abort()
					return
				}
			}
		}
		// RBAC：viewer 角色只允许 GET 方法（只读权限）。
		// 例外：账户自身安全管理（改密/绑定解绑 MFA/登出/个人信息）属于基本权利，
		// 不受只读限制——否则 viewer 被要求强制 MFA 时会死锁（绑不了 → 永远进不去）。
		if u.Role == "viewer" && c.Request.Method != "GET" && !selfServicePaths[c.FullPath()] {
			s.fail(c, http.StatusForbidden, 40304, "只读用户无操作权限")
			c.Abort()
			return
		}
		c.Set("operator", claims.Username)
		c.Set("userID", claims.UserID)
		c.Set("role", u.Role)
		// scope 供 /auth/me 回显：前端据此在刷新后仍能恢复「受限会话」守卫
		c.Set("scope", claims.Scope)
		c.Next()
	}
}
