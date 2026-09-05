package db

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
)

// utcNormalizer — SQLite 时区规范化存储（技术债清偿）。
//
// glebarez/go-sqlite 驱动没有时区 DSN 开关：绑定 time.Time 时按值自身 Location
// 格式化（"2026-09-05T12:00:00+08:00"），存储随实例时区漂移，且同表混存不同偏移
// 后 TEXT 词法比较（BETWEEN/ORDER BY created_at）会错序。本包装在 database/sql
// 边界把所有绑定参数中的 time.Time 归一化为**固定 3 位毫秒宽的 UTC 文本**
// （"2026-09-05T04:00:00.000Z"），物理存储恒为该形态，读回经驱动 parseTimeFormats
// 按 UTC 解析，语义与写库前一致。
//
// 拦截面：主 ConnPool + 事务 ConnPool（gorm Begin 会整体替换 tx.Statement.
// ConnPool，gorm 默认事务与显式 Transaction 都必须覆盖）；回调/Raw/Exec 全部
// 走同一 ConnPool，天然生效。MySQL（parseTime&loc=Local）/PG（timestamptz）不
// 使用此包装，由各自驱动保证。

// utcTextLayout 固定 3 位毫秒宽的 UTC 文本（"2026-09-05T04:00:00.000Z"）。
// 定宽保证 TEXT 词法序 == 时间序（RFC3339Nano 变宽小数会破坏 BETWEEN/ORDER BY）；
// 与迁移 strftime('%Y-%m-%dT%H:%M:%f')||'Z' 的输出同形，新旧行可直接比较。
const utcTextLayout = "2006-01-02T15:04:05.000Z07:00"

// normalizeArgs 含 time.Time 时返回参数的 UTC 定宽文本副本；否则原样返回（零拷贝）。
// gorm 主池 / Raw / 事务全部经过此包装，杜绝驱动双格式化（T 形 vs 空格形）分叉。
func normalizeArgs(args []interface{}) []interface{} {
	hasTime := false
	for _, a := range args {
		if _, ok := a.(time.Time); ok {
			hasTime = true
			break
		}
	}
	if !hasTime {
		return args
	}
	out := make([]interface{}, len(args))
	for i, a := range args {
		if t, ok := a.(time.Time); ok {
			out[i] = t.UTC().Format(utcTextLayout)
		} else {
			out[i] = a
		}
	}
	return out
}

// utcPool 包装 *sql.DB，满足 gorm.ConnPool + GetDBConnector（DB() 安全取底层句柄）。
// 必须以指针形态使用：gorm Commit/Rollback 对 TxCommitter 做 reflect.IsNil，
// 结构体值（非可 nil kind）会 panic。
type utcPool struct {
	db *sql.DB
}

func (p *utcPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, normalizeArgs(args)...)
}

func (p *utcPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, normalizeArgs(args)...)
}

func (p *utcPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.db.QueryRowContext(ctx, query, normalizeArgs(args)...)
}

func (p *utcPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, query)
}

func (p *utcPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &utcTx{tx}, nil
}

func (p *utcPool) Ping() error { return p.db.Ping() }

// GetDBConn 实现 gorm.GetDBConnector：gorm.DB() 优先走此接口取底层 *sql.DB，
// 避免 DB() 内部对非标准池做硬断言。
func (p *utcPool) GetDBConn() (*sql.DB, error) { return p.db, nil }

// utcTx 包装 *sql.Tx，事务内续拦（gorm.Begin 走 ConnPoolBeginner 分支接入）。
// Commit/Rollback 满足 gorm.TxCommitter，SavePoint 等语句经 ExecContext 透传。
type utcTx struct {
	tx *sql.Tx
}

func (t *utcTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, normalizeArgs(args)...)
}

func (t *utcTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, normalizeArgs(args)...)
}

func (t *utcTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, normalizeArgs(args)...)
}

func (t *utcTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, query)
}

func (t *utcTx) Commit() error   { return t.tx.Commit() }
func (t *utcTx) Rollback() error { return t.tx.Rollback() }
