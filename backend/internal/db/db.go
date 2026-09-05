// Package db 提供多数据库支持（PRD 6.3.1 / REQ NFR-016）：
// SQLite(纯 Go 驱动，免 CGO) / PostgreSQL / MySQL，统一 GORM 接口。
package db

import (
	"fmt"
	"log"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"llmrouter/internal/model"
)

// Open 依据 DB_TYPE / DB_DSN 打开数据库并执行首次自动迁移。
//   - sqlite   : DSN 为文件路径，如 ./gateway.db
//   - postgres : DSN 为 pg 连接串，如 postgres://user:pass@localhost:5432/gateway
//   - mysql    : DSN 兼容 go-sql-driver，如 user:pass@tcp(localhost:3306)/gateway?charset=utf8mb4&parseTime=True&loc=Local
func Open(dbType, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(dbType) {
	case "sqlite", "":
		dialector = sqlite.Open(dsn)
	case "postgres", "postgresql":
		dialector = postgres.Open(dsn)
	case "mysql":
		if !strings.Contains(dsn, "parseTime") {
			sep := "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
			dsn += sep + "charset=utf8mb4&parseTime=True&loc=Local"
		}
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q (sqlite|postgres|mysql)", dbType)
	}
	gdb, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		// SQLite 单写者，降低锁冲突
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open %s db: %w", dbType, err)
	}
	if strings.ToLower(dbType) == "sqlite" {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.SetMaxOpenConns(1) // 避免 sqlite 写锁竞争（WAL 下读仍并发）
		}
		// UTC 归一化池（第十五轮）：拦截所有绑定参数的 time.Time → UTC 存储。
		// 主池 + 根语句同步替换；Session/事务克隆自同一 Config，全路径生效。
		if sqlDB, err := gdb.DB(); err == nil {
			pool := &utcPool{db: sqlDB}
			gdb.Config.ConnPool = pool
			gdb.Statement.ConnPool = pool
		}
		gdb.Exec("PRAGMA journal_mode=WAL")
	}
	if err := gdb.AutoMigrate(model.AllModels()...); err != nil { // REQ-009/6.3.1 首次启动自动建表
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	if strings.ToLower(dbType) == "sqlite" {
		// 历史数据一次性迁移：本地偏移文本 → UTC（幂等，非 Z 值才改写）
		if err := normalizeSQLiteTimeColumns(gdb); err != nil {
			log.Printf("[sqlite-utc] normalize legacy rows: %v", err)
		}
	}
	return gdb, nil
}
