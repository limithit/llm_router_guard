package db

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type tzProbe struct {
	ID uint `gorm:"primaryKey"`
	At time.Time
}

func (tzProbe) TableName() string { return "tz_probes" }

func openUTCProbe(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gdb.AutoMigrate(&tzProbe{}); err != nil {
		t.Fatalf("migrate probe: %v", err)
	}
	return gdb
}

func storedAt(t *testing.T, gdb *gorm.DB) string {
	t.Helper()
	var stored string
	if err := gdb.Raw(`SELECT at FROM tz_probes LIMIT 1`).Scan(&stored).Error; err != nil {
		t.Fatalf("raw scan: %v", err)
	}
	return stored
}

func mustEqualInstant(t *testing.T, stored string, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("instant mismatch: stored=%q got=%s want=%s", stored, got, want)
	}
}

// localNoon 构造本地时区正午（远离午夜，避免日期翻转类误判）。
func localNoon() time.Time {
	return time.Date(2026, 9, 5, 12, 0, 0, 0, time.Local)
}

// TestSQLiteUTC_DefaultTransaction 默认事务（gorm 自动包裹 Create）路径：存 UTC "…Z"。
func TestSQLiteUTC_DefaultTransaction(t *testing.T) {
	gdb := openUTCProbe(t)
	local := localNoon()
	if err := gdb.Create(&tzProbe{At: local}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	stored := storedAt(t, gdb)
	if !strings.HasSuffix(stored, "Z") {
		t.Fatalf("default-tx path must store UTC '…Z', got %q", stored)
	}
	var back tzProbe
	if err := gdb.First(&back).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	mustEqualInstant(t, stored, back.At, local)
	if back.At.In(time.Local).Hour() != 12 {
		t.Errorf("local conversion lost: hour=%d", back.At.In(time.Local).Hour())
	}
}

// TestSQLiteUTC_ExplicitTransaction 显式 Transaction 路径（gorm.Begin 整体替换
// tx.Statement.ConnPool，必须走 utcPool→utcTx 包装）：存 UTC "…Z"。
func TestSQLiteUTC_ExplicitTransaction(t *testing.T) {
	gdb := openUTCProbe(t)
	local := localNoon()
	err := gdb.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&tzProbe{At: local}).Error
	})
	if err != nil {
		t.Fatalf("tx create: %v", err)
	}
	stored := storedAt(t, gdb)
	if !strings.HasSuffix(stored, "Z") {
		t.Fatalf("explicit-tx path must store UTC '…Z', got %q", stored)
	}
	var back tzProbe
	if err := gdb.First(&back).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	mustEqualInstant(t, stored, back.At, local)
}

// TestSQLiteUTC_RawBinding Raw/Exec 绑定 time.Time 参数也归一化（范围查询一致性）。
func TestSQLiteUTC_RawBinding(t *testing.T) {
	gdb := openUTCProbe(t)
	local := localNoon()
	if err := gdb.Exec(`INSERT INTO tz_probes (at) VALUES (?)`, local).Error; err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	if stored := storedAt(t, gdb); !strings.HasSuffix(stored, "Z") {
		t.Fatalf("raw binding must store UTC '…Z', got %q", stored)
	}
	// 归一化后的范围查询应能命中
	var n int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM tz_probes WHERE at >= ? AND at <= ?`,
		local.Add(-time.Hour), local.Add(time.Hour)).Scan(&n).Error; err != nil {
		t.Fatalf("range query: %v", err)
	}
	if n != 1 {
		t.Errorf("range query hits=%d, want 1", n)
	}
}

// TestSQLiteUTC_LegacyRewrite 历史数据迁移：本地偏移文本被改写为 UTC 且时刻不变，
// 已是 UTC 的行保持原样（幂等）。
// 注意：读路径会把"像时间"的 TEXT 自动转为 time.Time 再按 RFC3339Nano 重格式化
// （.000 等零小数被裁剪），故文本级断言一律走服务端 LIKE/julianday（免疫读转换）。
func TestSQLiteUTC_LegacyRewrite(t *testing.T) {
	gdb := openUTCProbe(t)
	legacyLocal := "2026-09-05T12:00:00.000+08:00" // 修复前驱动形态（本地 +08:00）
	legacyUTC := "2026-09-05T04:00:00.000Z"        // 已规范行
	if err := gdb.Exec(`INSERT INTO tz_probes (id, at) VALUES (1, ?)`, legacyLocal).Error; err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := gdb.Exec(`INSERT INTO tz_probes (id, at) VALUES (2, ?)`, legacyUTC).Error; err != nil {
		t.Fatalf("seed utc: %v", err)
	}
	if err := normalizeSQLiteTimeColumns(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 再次执行验证幂等
	if err := normalizeSQLiteTimeColumns(gdb); err != nil {
		t.Fatalf("migrate again: %v", err)
	}

	// 服务端文本级断言：本地偏移清零、全部 Z 结尾
	var nOffset, nZ int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM tz_probes WHERE at LIKE '%+08:00'`).Scan(&nOffset).Error; err != nil {
		t.Fatalf("count offset: %v", err)
	}
	if err := gdb.Raw(`SELECT COUNT(*) FROM tz_probes WHERE at LIKE '%Z'`).Scan(&nZ).Error; err != nil {
		t.Fatalf("count z: %v", err)
	}
	if nOffset != 0 {
		t.Errorf("local-offset rows remain: %d", nOffset)
	}
	if nZ != 2 {
		t.Errorf("Z-terminated rows=%d, want 2", nZ)
	}

	// 时刻等价断言（服务端 julianday 对比）：两行都等于 UTC 04:00
	var nInstant int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM tz_probes WHERE julianday(at) = julianday(?)`,
		"2026-09-05T04:00:00Z").Scan(&nInstant).Error; err != nil {
		t.Fatalf("count instant: %v", err)
	}
	if nInstant != 2 {
		t.Errorf("instant-matched rows=%d, want 2", nInstant)
	}

	// 迁移后历史行可被归一化范围查询命中
	var n int64
	base := time.Date(2026, 9, 5, 4, 0, 0, 0, time.UTC)
	if err := gdb.Raw(`SELECT COUNT(*) FROM tz_probes WHERE at BETWEEN ? AND ?`,
		base.Add(-time.Minute), base.Add(time.Minute)).Scan(&n).Error; err != nil {
		t.Fatalf("range after migrate: %v", err)
	}
	if n != 2 {
		t.Errorf("range hits=%d, want 2 (legacy+utc rows)", n)
	}

	// gorm 读回实例等价（+08:00 12:00 == UTC 04:00）
	want := time.Date(2026, 9, 5, 12, 0, 0, 0, time.FixedZone("+0800", 8*3600))
	var back tzProbe
	if err := gdb.First(&back, 1).Error; err != nil {
		t.Fatalf("read back legacy: %v", err)
	}
	if !back.At.Equal(want) {
		t.Errorf("read-back instant mismatch: got %s want %s", back.At, want)
	}
}
