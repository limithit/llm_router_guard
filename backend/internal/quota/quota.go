// Package quota 实现速率限制（短周期，REQ-013）与配额（长周期，REQ-012）。
// 限流计数在内存（固定窗口）；配额用量以数据库为准（跨重启持久），
// 周期到期自动/惰性重置（REQ-012 定时自动重置 + 手动重置）。
package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/httpclient"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
)

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// ---------- 速率限制：固定窗口计数器 ----------

// Limiter 限流判定接口：内存版（*RateLimiter，单节点）与 Redis 分布式版
// （*RedisLimiter，多节点）实现同一签名，部署形态决定注入哪个实现。
type Limiter interface {
	Allow(snap *runtime.Snapshot, apiKeyID uint, alias string) (bool, *model.RateLimitRule)
	FlushHits()
}

type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*window // key = ruleID:apiKeyID:alias
	hits    map[uint]int64     // 规则命中(拒绝)次数，定时落库
	db      *gorm.DB
}

type window struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(gdb *gorm.DB) *RateLimiter {
	return &RateLimiter{db: gdb, windows: map[string]*window{}, hits: map[uint]int64{}}
}

// Allow 依据快照中的限流规则判断是否放行；返回 false 时给出命中规则。
func (rl *RateLimiter) Allow(snap *runtime.Snapshot, apiKeyID uint, alias string) (bool, *model.RateLimitRule) {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for i := range snap.RateLimits {
		r := &snap.RateLimits[i]
		if r.APIKeyID != 0 && r.APIKeyID != apiKeyID {
			continue
		}
		if r.ModelAlias != "" && r.ModelAlias != "*" && r.ModelAlias != alias {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s", r.ID, apiKeyID, alias)
		w, ok := rl.windows[key]
		if !ok || now.After(w.resetAt) {
			w = &window{count: 0, resetAt: now.Add(time.Duration(max1(r.WindowSeconds)) * time.Second)}
			rl.windows[key] = w
		}
		w.count++
		if w.count > max1(r.MaxRequests) {
			rl.hits[r.ID]++
			return false, r
		}
	}
	return true, nil
}

// FlushHits 将累计拒绝次数写回 rate_limit_rules.total_hits（每分钟）。
func (rl *RateLimiter) FlushHits() {
	rl.mu.Lock()
	m := rl.hits
	rl.hits = map[uint]int64{}
	rl.mu.Unlock()
	for id, n := range m {
		rl.db.Model(&model.RateLimitRule{}).Where("id = ?", id).
			UpdateColumn("total_hits", gorm.Expr("total_hits + ?", n))
	}
}

// ---------- 配额管理 ----------

type QuotaManager struct {
	db *gorm.DB
}

func NewQuotaManager(gdb *gorm.DB) *QuotaManager { return &QuotaManager{db: gdb} }

// NextReset 计算下一周期终点（本地时区）。
func NextReset(period string, from time.Time) time.Time {
	switch period {
	case "week":
		// 以周一为一周起点（ISO/中国习惯）：把 Go 的 Weekday（周日=0..周六=6）
		// 转成"距周一的天数"（周一=0..周日=6），再算到下个周一。
		m := (int(from.Weekday()) + 6) % 7
		d := (7 - m) % 7
		if d == 0 {
			d = 7
		}
		return time.Date(from.Year(), from.Month(), from.Day()+d, 0, 0, 0, 0, from.Location())
	case "month":
		return time.Date(from.Year(), from.Month()+1, 1, 0, 0, 0, 0, from.Location())
	default: // day
		return time.Date(from.Year(), from.Month(), from.Day()+1, 0, 0, 0, 0, from.Location())
	}
}

// matchQuotas 找出作用于 (apiKeyID, alias) 的启用配额下标。
func matchQuotas(snap *runtime.Snapshot, apiKeyID uint, alias string) []*model.Quota {
	var out []*model.Quota
	for i := range snap.Quotas {
		q := &snap.Quotas[i]
		if q.APIKeyID != 0 && q.APIKeyID != apiKeyID {
			continue
		}
		if q.ModelAlias != "*" && q.ModelAlias != alias {
			continue
		}
		out = append(out, q)
	}
	return out
}

// OverInfo 超限详情。
type OverInfo struct {
	QuotaID      uint
	OverAction   string // reject|degrade
	DegradeAlias string
	Kind         string
	Used, Limit  int64
}

// Check 以 DB 即时值复核配额；全部未超限返回 true。
// 任一超限即返回对应 OverInfo（首条命中生效）。
func (qm *QuotaManager) Check(snap *runtime.Snapshot, apiKeyID uint, alias string) (bool, *OverInfo) {
	qs := matchQuotas(snap, apiKeyID, alias)
	if len(qs) == 0 {
		return true, nil
	}
	ids := make([]uint, 0, len(qs))
	for _, q := range qs {
		ids = append(ids, q.ID)
	}
	var rows []model.Quota
	qm.db.Where("id IN ? AND enabled = ?", ids, true).Find(&rows)
	now := time.Now()
	byID := map[uint]model.Quota{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, q := range qs { // 按快照顺序（创建序）判定
		r, ok := byID[q.ID]
		if !ok {
			continue
		}
		used := r.UsedValue
		if now.After(r.ResetAt) { // 周期已过：视同已重置
			used = 0
		}
		if r.LimitValue > 0 && used >= r.LimitValue {
			return false, &OverInfo{QuotaID: r.ID, OverAction: r.OverAction, DegradeAlias: r.DegradeAlias,
				Kind: r.QuotaType, Used: used, Limit: r.LimitValue}
		}
	}
	return true, nil
}

// ---------- 预扣-结算（SEC-07：封死 Check→Consume 竞态窗口） ----------
// 旧模型：转发前只读 Check、转发后无条件 Consume —— 并发请求全部通过 Check 后
// 集体超发，限额形同虚设；且失败流不计量。新模型：
//   Reserve：转发前对每条命中配额做「条件原子自增」（used+est<=limit 才成功），
//            任一失败回滚本批预扣并返回超限（可触发降级重试）；
//   Settle ：完成后按实际用量与预估之差回补/退还（requests 恒保留 1）；
//   Release：全链路失败（上游不可达）时全额退还。
// 残余超发被压缩到「单请求预估值」量级；跨进程多实例场景仍可能各自预扣
// （sqlite 单写锁下窗口极小），生产多节点建议共享 DB/Redis。

type reservedItem struct {
	quotaID uint
	kind    string
	amount  int64
}

// Reservation 预扣回执（Settle/Release 幂等）。
type Reservation struct {
	items  []reservedItem
	closed bool
}

// Reserve 原子预扣；超限返回 (nil, over)。estTokens 为完成 token 预估（含 prompt）。
func (qm *QuotaManager) Reserve(snap *runtime.Snapshot, apiKeyID uint, alias string, estTokens int64) (*Reservation, *OverInfo) {
	qs := matchQuotas(snap, apiKeyID, alias)
	res := &Reservation{}
	if len(qs) == 0 {
		return res, nil
	}
	now := time.Now()
	for _, q := range qs {
		var amount int64
		switch q.QuotaType {
		case "requests":
			amount = 1
		case "tokens":
			amount = estTokens
		default:
			continue
		}
		if amount <= 0 {
			continue
		}
		// 周期过期的惰性重置（带条件，重复执行安全）
		if now.After(q.ResetAt) {
			qm.db.Model(&model.Quota{}).Where("id = ? AND reset_at <= ?", q.ID, now).
				Updates(map[string]any{"used_value": 0, "reset_at": NextReset(q.Period, now)})
		}
		var upd *gorm.DB
		if q.LimitValue > 0 {
			upd = qm.db.Model(&model.Quota{}).
				Where("id = ? AND enabled = ? AND (limit_value = 0 OR used_value + ? <= limit_value)", q.ID, true, amount).
				UpdateColumn("used_value", gorm.Expr("used_value + ?", amount))
		} else {
			upd = qm.db.Model(&model.Quota{}).Where("id = ? AND enabled = ?", q.ID, true).
				UpdateColumn("used_value", gorm.Expr("used_value + ?", amount))
		}
		if upd.Error != nil || upd.RowsAffected == 0 {
			qm.Release(res) // 回滚已完成的行级预扣
			over := &OverInfo{QuotaID: q.ID, OverAction: q.OverAction, DegradeAlias: q.DegradeAlias, Kind: q.QuotaType}
			var fresh model.Quota
			if qm.db.Select("used_value", "limit_value").First(&fresh, q.ID).Error == nil {
				over.Used, over.Limit = fresh.UsedValue, fresh.LimitValue
			}
			return nil, over
		}
		res.items = append(res.items, reservedItem{quotaID: q.ID, kind: q.QuotaType, amount: amount})
		qm.maybeAlert(q, amount)
	}
	return res, nil
}

// Settle 按实际 token 用量与预扣之差回补/退还；requests 保留（请求确实发生过）。
func (qm *QuotaManager) Settle(res *Reservation, actualTokens int64) {
	if res == nil || res.closed {
		return
	}
	res.closed = true
	for _, it := range res.items {
		if it.kind != "tokens" {
			continue
		}
		delta := actualTokens - it.amount
		if delta == 0 {
			continue
		}
		qm.db.Model(&model.Quota{}).Where("id = ?", it.quotaID).
			UpdateColumn("used_value",
				gorm.Expr("CASE WHEN used_value + ? < 0 THEN 0 ELSE used_value + ? END", delta, delta))
	}
}

// Release 全额退还（含 requests）：上游完全不可达、零成本场景。
func (qm *QuotaManager) Release(res *Reservation) {
	if res == nil || res.closed {
		return
	}
	res.closed = true
	for _, it := range res.items {
		qm.db.Model(&model.Quota{}).Where("id = ?", it.quotaID).
			UpdateColumn("used_value",
				gorm.Expr("CASE WHEN used_value - ? < 0 THEN 0 ELSE used_value - ? END", it.amount, it.amount))
	}
}

// Consume 请求完成后累加用量：requests +1；tokens +prompt+completion。
// 周期过期时先重置再累加（惰性重置），并检查预警阈值（REQ-014）。
// 注：网关热路径已改用 Reserve/Settle（SEC-07）；此方法保留给统计校正类调用。
func (qm *QuotaManager) Consume(snap *runtime.Snapshot, apiKeyID uint, alias string, promptTokens, completionTokens int) {
	for _, q := range matchQuotas(snap, apiKeyID, alias) {
		var delta int64
		switch q.QuotaType {
		case "requests":
			delta = 1
		case "tokens":
			delta = int64(promptTokens + completionTokens)
		}
		if delta <= 0 {
			continue
		}
		now := time.Now()
		if now.After(q.ResetAt) {
			qm.db.Model(&model.Quota{}).Where("id = ?", q.ID).Updates(map[string]any{
				"used_value": delta, "reset_at": NextReset(q.Period, now)})
		} else {
			qm.db.Model(&model.Quota{}).Where("id = ?", q.ID).
				UpdateColumn("used_value", gorm.Expr("used_value + ?", delta))
		}
		qm.maybeAlert(q, delta)
	}
}

// maybeAlert 跨过预警阈值时触发 Webhook（尽力而为，不阻断请求）。
func (qm *QuotaManager) maybeAlert(q *model.Quota, delta int64) {
	var s settings.QuotaAlerts
	_ = settings.LoadKV(qm.db, model.SetKeyQuotaAlert, &s, settings.DefaultQuotaAlerts())
	if !s.Enabled || q.LimitValue <= 0 {
		return
	}
	before := float64(q.UsedValue) / float64(q.LimitValue) * 100
	after := float64(q.UsedValue+delta) / float64(q.LimitValue) * 100
	for _, th := range s.Thresholds {
		if before < float64(th) && after >= float64(th) {
			if s.Channel == "webhook" && s.WebhookURL != "" {
				payload := map[string]any{"level": "quota_alert", "quota_id": q.ID,
					"threshold": th, "message": fmt.Sprintf("配额#%d（Key#%d/%s/%s）使用率达到 %d%%",
						q.ID, q.APIKeyID, q.ModelAlias, q.Period, th)}
				go postWebhook(s.WebhookURL, payload)
			}
			return
		}
	}
}

func postWebhook(url string, payload any) {
	b, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// M-01：webhook 同样拒绝重定向跟随（管理端可配 URL，不引入 302 信道）
	client := httpclient.New(6 * time.Second)
	if resp, err := client.Do(req); err == nil {
		resp.Body.Close()
	}
}

// StartBackground 定时任务：过期配额重置 + 限流命中统计落库。
func (qm *QuotaManager) StartBackground(ctx context.Context, rl Limiter) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				for _, period := range []string{"day", "week", "month"} {
					var rows []model.Quota
					qm.db.Where("period = ? AND reset_at <= ?", period, now).Find(&rows)
					for _, r := range rows {
						qm.db.Model(&model.Quota{}).Where("id = ?", r.ID).Updates(map[string]any{
							"used_value": 0, "reset_at": NextReset(period, now)})
					}
				}
				rl.FlushHits()
			}
		}
	}()
}
