// users.go 用户 MFA 状态管理（REQ-023）：列表 / 强制解绑 / 解锁。
package admin

import (
	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
)

type userRow struct {
	ID          uint    `json:"id"`
	Username    string  `json:"username"`
	MFAEnabled  bool    `json:"mfa_enabled"`
	LastLoginAt *string `json:"last_login_at"`
	Locked      bool    `json:"locked"`
	CreatedAt   *string `json:"created_at,omitempty"`
}

func (s *Server) listUsers(c *gin.Context) {
	page, size := parsePage(c)
	var total int64
	s.db.Model(&model.AdminUser{}).Count(&total)
	var users []model.AdminUser
	s.db.Order("id").Offset((page - 1) * size).Limit(size).Find(&users)
	out := make([]userRow, 0, len(users))
	for _, u := range users {
		row := userRow{ID: u.ID, Username: u.Username, MFAEnabled: u.MFAEnabled, Locked: u.IsLocked()}
		if u.LastLoginAt != nil {
			t := u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
			row.LastLoginAt = &t
		}
		out = append(out, row)
	}
	okPaged(c, out, int(total), page, size)
}

func (s *Server) unbindUserMfa(c *gin.Context) {
	id := c.Param("id")
	var u model.AdminUser
	if err := s.db.First(&u, id).Error; err != nil {
		s.fail(c, 404, 40401, "用户不存在")
		return
	}
	if !u.MFAEnabled {
		s.fail(c, 409, 40901, "该用户未绑定 MFA")
		return
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Updates(map[string]any{
		"mfa_secret": "", "mfa_enabled": false, "mfa_bound_at": nil,
		"recovery_codes_json": "", "last_totp_step": 0})
	// SEC-13：被解绑者既有的全部会话立即作废（防止带 MFA 语义的旧 Token 继续通行）
	s.invalidateSessions(u.ID)
	s.recordOp(c, "mfa_unbind", "security", u.Username, gin.H{"mfa_enabled": true}, gin.H{"mfa_enabled": false})
	s.ok(c, nil)
}

func (s *Server) unlockUser(c *gin.Context) {
	id := c.Param("id")
	var u model.AdminUser
	if err := s.db.First(&u, id).Error; err != nil {
		s.fail(c, 404, 40401, "用户不存在")
		return
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		Updates(map[string]any{"failed_logins": 0, "locked_until": nil})
	s.recordOp(c, "update", "security", u.Username+"#unlock", nil, gin.H{"locked": false})
	s.ok(c, nil)
}
