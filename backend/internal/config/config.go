// Package config 负责启动级环境变量（数据库连接、端口、密钥等）。
// 注意：业务配置全部存数据库（PRD 6.2 配置存储），这里只放引导参数。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenPort      int
	DBType          string // sqlite|postgres|mysql
	DBDSN           string
	JWTSecret       string
	MasterKey       string // 供应商 API Key 加密主密钥
	AdminUser       string // 首启引导管理员
	AdminPassword   string
	FrontendDist    string // 前端构建产物目录（可选内嵌服务）
	DataDir         string // 备份文件目录
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	TrustedProxies  []string // 可信反向代理 CIDR；空=不信任任何 X-Forwarded-For，ClientIP 取 TCP 对端
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// splitCSV 按逗号拆分并去空白/空项。
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func Load() *Config {
	port, _ := strconv.Atoi(getenv("PORT", "8080"))
	return &Config{
		ListenPort:    port,
		DBType:        getenv("DB_TYPE", "sqlite"),
		DBDSN:         getenv("DB_DSN", "gateway.db"),
		JWTSecret:     getenv("JWT_SECRET", "change-me-jwt-secret"),
		MasterKey:     getenv("MASTER_KEY", "llm-router-guard-master-key"),
		AdminUser:     getenv("ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		FrontendDist:  getenv("FRONTEND_DIST", ""),
		DataDir:       getenv("DATA_DIR", "./data"),
		ReadTimeout:    90 * time.Second,
		WriteTimeout:   0, // SSE 长连接不设总写超时，由逐请求 context 控制
		TrustedProxies: splitCSV(getenv("TRUSTED_PROXIES", "")),
	}
}
