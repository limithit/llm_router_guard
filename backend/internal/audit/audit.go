// Package audit 提供调用日志异步批量写入与操作审计记录（REQ-003/015），
// 并按保留天数自动清理（REQ-001 ③）。
package audit

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/model"
)

type Logger struct {
	db    *gorm.DB
	ch    chan *model.CallLog
	clean func() time.Duration // 审计保留期
}

func NewLogger(gdb *gorm.DB, retention func() time.Duration) *Logger {
	return &Logger{db: gdb, ch: make(chan *model.CallLog, 4096), clean: retention}
}

func (l *Logger) Start(ctx context.Context) {
	go l.writerLoop(ctx)
	go l.cleanupLoop(ctx)
}

// Write 非阻塞投递调用日志；队列满时直接落库兜底（不丢审计）。
func (l *Logger) Write(cl *model.CallLog) {
	if cl == nil {
		return
	}
	select {
	case l.ch <- cl:
	default:
		l.db.Create(cl)
	}
}

func (l *Logger) writerLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	batch := make([]*model.CallLog, 0, 64)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.db.CreateInBatches(batch, 50).Error; err != nil {
			log.Printf("[audit] batch insert failed: %v", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case cl := <-l.ch:
			batch = append(batch, cl)
			if len(batch) >= 64 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// cleanupLoop 每小时按保留天数清理调用/操作审计（REQ-001 ③）。
func (l *Logger) cleanupLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			days := int(l.clean().Hours() / 24)
			if days <= 0 {
				days = 90
			}
			cutoff := time.Now().AddDate(0, 0, -days)
			l.db.Where("created_at < ?", cutoff).Delete(&model.CallLog{})
			l.db.Where("created_at < ?", cutoff).Delete(&model.OperationLog{})
		}
	}
}

// LogOp 记录一条操作审计（REQ-003：操作人/时间/类型/模块/前后对比/IP/生效状态）。
func LogOp(gdb *gorm.DB, operator, action, module, target string, before, after any, ip string, effective bool) {
	mj := func(v any) string {
		if v == nil {
			return ""
		}
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
	gdb.Create(&model.OperationLog{
		CreatedAt: time.Now(), Operator: operator, Action: action, Module: module,
		Target: target, BeforeJSON: mj(before), AfterJSON: mj(after), IP: ip, Effective: effective,
	})
}
