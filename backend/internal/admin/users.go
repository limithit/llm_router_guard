// users.go 管理员用户管理：列表 / 创建 / 改密 / 删除 / 强制解绑 MFA / 解锁。
package admin

import (
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"llmrouter/internal/model"
)

type userRow struct {
	ID          uint    `json:"id"`
	Username    string  `json:"username"`
	Role        string  `json:"role"`
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
		row := userRow{ID: u.ID, Username: u.Username, Role: u.Role, MFAEnabled: u.MFAEnabled, Locked: u.IsLocked()}
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

// createUserReq 创建管理员请求体。
type createUserReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// Role: "admin"（全权限，默认）或 "viewer"（只读，仅允许 GET）。
	Role string `json:"role,omitempty"`
	// ForceChangePassword=true 则新建用户首登强制改密（安全默认）。
	ForceChangePassword *bool `json:"force_change_password,omitempty"`
}

// createUser 创建新管理员（仅认证管理员可调）。
func (s *Server) createUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, 400, 40001, "参数错误："+err.Error())
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(username) > 64 {
		s.fail(c, 400, 40001, "用户名须 1-64 字符")
		return
	}
	if len(req.Password) < 8 {
		s.fail(c, 400, 40001, "密码至少 8 位")
		return
	}
	forceChange := true
	if req.ForceChangePassword != nil {
		forceChange = *req.ForceChangePassword
	}
	// 角色校验：默认 admin，仅允许 admin / viewer
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "admin"
	}
	if role != "admin" && role != "viewer" {
		s.fail(c, 400, 40001, "角色必须为 admin 或 viewer")
		return
	}
	// 重名检查
	var cnt int64
	s.db.Model(&model.AdminUser{}).Where("username = ?", username).Count(&cnt)
	if cnt > 0 {
		s.fail(c, 409, 40901, "用户名已存在")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.fail(c, 500, 50000, "口令哈希失败")
		return
	}
	u := model.AdminUser{
		Username:           username,
		PasswordHash:       string(hash),
		Role:               role,
		MustChangePassword: forceChange,
	}
	if err := s.db.Create(&u).Error; err != nil {
		s.fail(c, 500, 50000, "创建用户失败："+err.Error())
		return
	}
	s.recordOp(c, "create", "security", username, nil, gin.H{"id": u.ID, "role": role, "force_change": forceChange})
	s.ok(c, gin.H{"id": u.ID, "username": u.Username, "role": role, "must_change_password": forceChange})
}

// changeUserPasswordReq 改密请求体。
type changeUserPasswordReq struct {
	Password string `json:"password" binding:"required"`
	// ForceChangePassword=true 则置 must_change_password（给新建用户改临时密码时用）。
	ForceChangePassword *bool `json:"force_change_password,omitempty"`
}

// changeUserPassword 管理员修改另一管理员的口令（不含旧密码校验——是管理员特权操作）。
func (s *Server) changeUserPassword(c *gin.Context) {
	id := c.Param("id")
	var u model.AdminUser
	if err := s.db.First(&u, id).Error; err != nil {
		s.fail(c, 404, 40401, "用户不存在")
		return
	}
	var req changeUserPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, 400, 40001, "参数错误："+err.Error())
		return
	}
	if len(req.Password) < 8 {
		s.fail(c, 400, 40001, "密码至少 8 位")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.fail(c, 500, 50000, "口令哈希失败")
		return
	}
	forceChange := false
	if req.ForceChangePassword != nil {
		forceChange = *req.ForceChangePassword
	}
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Updates(map[string]any{
		"password_hash":        string(hash),
		"must_change_password": forceChange,
		"failed_logins":        0,
		"locked_until":         nil,
	})
	// 改密即作废该用户既有全部会话
	s.invalidateSessions(u.ID)
	s.recordOp(c, "update", "security", u.Username+"#password", nil, gin.H{"force_change": forceChange})
	s.ok(c, nil)
}

// deleteUser 删除管理员（不可删除自己、不可删除最后一个管理员）。
func (s *Server) deleteUser(c *gin.Context) {
	id := c.Param("id")
	var u model.AdminUser
	if err := s.db.First(&u, id).Error; err != nil {
		s.fail(c, 404, 40401, "用户不存在")
		return
	}
	// 不允许删自己
	if uid, ok := c.Get("userID"); ok && uid.(uint) == u.ID {
		s.fail(c, 409, 40901, "不可删除当前登录用户")
		return
	}
	// 不允许删最后一个管理员（防止锁死）
	var total int64
	s.db.Model(&model.AdminUser{}).Count(&total)
	if total <= 1 {
		s.fail(c, 409, 40902, "至少保留一个管理员账户")
		return
	}
	// 如果被删用户绑定了 MFA，先清密钥（安全残留清零）
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).Updates(map[string]any{
		"mfa_secret": "", "mfa_enabled": false, "recovery_codes_json": ""})
	// 作废其会话
	s.invalidateSessions(u.ID)
	if err := s.db.Delete(&u).Error; err != nil {
		s.fail(c, 500, 50000, "删除失败："+err.Error())
		return
	}
	s.recordOp(c, "delete", "security", u.Username, gin.H{"id": u.ID}, nil)
	s.ok(c, nil)
}

// changeUserRoleReq 改角色请求体。
type changeUserRoleReq struct {
	Role string `json:"role" binding:"required"`
}

// changeUserRole 修改管理员角色（admin ↔ viewer）。
func (s *Server) changeUserRole(c *gin.Context) {
	id := c.Param("id")
	var u model.AdminUser
	if err := s.db.First(&u, id).Error; err != nil {
		s.fail(c, 404, 40401, "用户不存在")
		return
	}
	var req changeUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, 400, 40001, "参数错误："+err.Error())
		return
	}
	role := strings.TrimSpace(req.Role)
	if role != "admin" && role != "viewer" {
		s.fail(c, 400, 40001, "角色必须为 admin 或 viewer")
		return
	}
	if u.Role == role {
		s.ok(c, nil)
		return
	}
	oldRole := u.Role
	s.db.Model(&model.AdminUser{}).Where("id = ?", u.ID).
		UpdateColumn("role", role)
	// 角色变更即作废既有会话（新角色需要重新登录获取新 token）
	s.invalidateSessions(u.ID)
	s.recordOp(c, "update", "security", u.Username+"#role", gin.H{"role": oldRole}, gin.H{"role": role})
	s.ok(c, nil)
}
