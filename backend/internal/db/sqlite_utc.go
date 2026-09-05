package db

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// sqliteUTCMigrate — 历史数据一次性迁移（第十五轮技术债配套）。
//
// 背景：修复前 glebarez 驱动按值自身时区写库（如 "2026-09-05T12:00:00+08:00"），
// 老库中存在本地偏移文本。本函数枚举库中真实表（sqlite_master + PRAGMA
// table_info，不依赖模型注册表，覆盖历史遗留表），对 TEXT 亲和列中的
// "ISO 日期样、非 Z 结尾" 值经 SQLite 日期函数改写为 UTC 文本。
//
// 防误伤：列须为 TEXT/DATETIME 声明类型；值须形如 "____-__-__%" 且
// julianday(x) 可解析（排除 NULL/空串/数字列/任意文本）。
// 幂等：仅命中非 Z 值，重复执行零改动；空表零开销。
// 失败逐列告警不阻断启动（历史值仍可被驱动正确解析，仅存在新旧混存）。

// normalizeSQLiteTimeColumns 把 sqlite 库中所有日期文本列的本地偏移值改写为 UTC。
func normalizeSQLiteTimeColumns(gdb *gorm.DB) error {
	var tables []string
	if err := gdb.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	).Scan(&tables).Error; err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	total := int64(0)
	for _, table := range tables {
		var cols []struct {
			Name string
			Type string
		}
		if err := gdb.Raw(fmt.Sprintf(`PRAGMA table_info(%q)`, table)).Scan(&cols).Error; err != nil {
			log.Printf("[sqlite-utc] table_info %s: %v", table, err)
			continue
		}
		for _, c := range cols {
			decl := strings.ToUpper(c.Type)
			if !strings.Contains(decl, "TEXT") && !strings.Contains(decl, "DATE") {
				continue // 仅日期文本列（gorm sqlite 的 time.Time 建为 datetime 亲和 TEXT）
			}
			q := fmt.Sprintf(
				`UPDATE %q SET %q = strftime('%%Y-%%m-%%dT%%H:%%M:%%f', %q) || 'Z'
				 WHERE %q IS NOT NULL AND %q <> '' AND %q NOT LIKE '%%Z'
				   AND %q LIKE '____-__-__%%' AND julianday(%q) IS NOT NULL`,
				table, c.Name, c.Name, c.Name, c.Name, c.Name, c.Name, c.Name)
			res := gdb.Exec(q)
			if res.Error != nil {
				log.Printf("[sqlite-utc] migrate %s.%s: %v", table, c.Name, res.Error)
				continue
			}
			total += res.RowsAffected
		}
	}
	if total > 0 {
		log.Printf("[sqlite-utc] legacy rows normalized to UTC: %d", total)
	}
	return nil
}
