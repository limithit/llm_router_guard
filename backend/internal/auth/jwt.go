// Package auth 管理后台 JWT 签发/校验（REQ：管理后台登录态）。
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"uname"`
	// Scope 受限会话标记（SEC-02/SEC-09）："" = 全权；"pw" = 仅允许改密；"mfa" = 仅允许绑定 MFA。
	// 中间件按 scope 强制路由白名单，前端无法绕过（此前 need_bind_mfa 仅是建议位）。
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

func Issue(secret string, userID uint, username string) (string, error) {
	return IssueScoped(secret, userID, username, "")
}

// IssueScoped 签发带 scope 限制的短期会话（2h），到期或完成引导动作后必须重新登录。
func IssueScoped(secret string, userID uint, username, scope string) (string, error) {
	ttl := 24 * time.Hour
	if scope != "" {
		ttl = 2 * time.Hour
	}
	claims := Claims{
		UserID: userID, Username: username, Scope: scope,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "llm-router-guard",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func Parse(secret, tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
