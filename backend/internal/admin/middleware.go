// middleware.go 管理 API 鉴权中间件（JWT）+ 速率辅助。
package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/auth"
)

// AuthMiddleware 校验 Authorization: Bearer <JWT>，写入 operator/userID。
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
		c.Set("operator", claims.Username)
		c.Set("userID", claims.UserID)
		c.Next()
	}
}
