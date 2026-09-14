// Package slb 实现加权轮询负载均衡与熔断（REQ-006/007）。
// 负载策略为平滑加权轮询（SWRR, smooth weighted round-robin）：按别名维护
// currentWeight 游标，确定性分发、不聚堆，比加权随机更可预期。
//
// 熔断状态存储（第九轮后续，多节点）：
//   - 失败计数按实例本地维护（语义不变）；熔断「打开」事件广播到 Redis
//     （SETEX，TTL=熔断重置秒数，到期键消失即自动半开），实例判定时以
//     ≤1 次/秒/供应商的节流读取共享打开状态 —— 任一实例打开熔断，
//     其余实例 ≤1s 内同步跳过该供应商；成功恢复即删除共享键。
//   - 未注入 Redis（单节点 / SQLite）时完全走内存，行为与旧版一致；
//     Redis 故障自动降级内存并后台探活恢复，不阻塞转发热路径。
//
// 全局 SWRR 游标（第十五轮 #16，多节点）：
//   - Redis 可用时，SWRR 的每一步游标推进改为 Lua 原子执行
//     （gw:swrr:v1:<alias> HASH：providerID → currentWeight），多实例
//     轮询序列互不重叠、严格按权重比例分流；
//   - 仅在"候选 = 全部上游且无 tried 排除"的干净路径走全局游标
//     （故障转移重试路径候选动态，本地游标兜底）；
//   - Redis 故障自动降级实例本地游标并后台探活恢复（同熔断共享语义）。
package slb

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
)

type breaker struct {
	mu        sync.Mutex
	fails     int
	openUntil time.Time
	lastErr   string
	lastFail  time.Time
	// 分布式共享状态缓存（SetRedis 后启用）：
	remoteOpenUntilMS int64     // 最近一次读到的共享「打开截止」(unix ms, 0=无)
	remoteCheckedAt   time.Time // 节流读：≤1 次/秒/供应商
}

// Balancer 从别名上游集合中按权重轮询挑选供应商，支持故障转移时排除已尝试项。
type Balancer struct {
	mu       sync.Mutex
	breakers map[uint]*breaker
	// rr: 别名 → (providerID → currentWeight) 游标，SWRR 状态（实例本地）。
	rr map[string]map[uint]int
	// 分布式熔断（多节点）：rdb 非空时启用；故障降级内存并探活恢复。
	rdb       *redis.Client
	redisDown atomicFlag
}

type atomicFlag struct{ v int32 }

func (f *atomicFlag) set(v bool) {
	if v {
		f.v = 1
	} else {
		f.v = 0
	}
}
func (f *atomicFlag) get() bool { return f.v == 1 }

func New() *Balancer {
	return &Balancer{breakers: map[uint]*breaker{}, rr: map[string]map[uint]int{}}
}

// SetRedis 启用 Redis 共享熔断打开状态（多节点）。连接失败不 panic：
// 打印告警并降级内存模式，后台探活恢复后自动切回。
func (bl *Balancer) SetRedis(addr, password string) {
	bl.rdb = redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DialTimeout:  500 * time.Millisecond,
		ReadTimeout:  300 * time.Millisecond,
		WriteTimeout: 300 * time.Millisecond,
		PoolSize:     20,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bl.rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[slb] WARNING: redis %s unreachable (%v); circuit state starts DEGRADED (per-instance) until it recovers", addr, err)
		bl.redisDown.set(true)
		go bl.watchRedis()
	} else {
		log.Printf("[slb] redis %s connected: shared circuit-breaker state ENABLED", addr)
	}
}

// watchRedis 降级期间探活，恢复后清除标记。
func (bl *Balancer) watchRedis() {
	for {
		time.Sleep(5 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := bl.rdb.Ping(ctx).Err()
		cancel()
		if err == nil {
			bl.redisDown.set(false)
			log.Printf("[slb] redis recovered: shared circuit-breaker state re-enabled")
			return
		}
	}
}

// cbKey 供应商熔断共享键：值为「打开截止」unix 毫秒，TTL=剩余打开时长。
func cbKey(id uint) string { return fmt.Sprintf("gw:cb:v1:%d", id) }

// remoteOpen 读取共享「打开截止」（unix ms；0=未打开/不可用）。故障时置降级标记。
func (bl *Balancer) remoteOpen(id uint) int64 {
	if bl.rdb == nil || bl.redisDown.get() {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	v, err := bl.rdb.Get(ctx, cbKey(id)).Int64()
	switch {
	case err == redis.Nil:
		return 0
	case err != nil:
		log.Printf("[slb] redis read circuit state: %v (degrading)", err)
		bl.redisDown.set(true)
		go bl.watchRedis()
		return 0
	}
	return v
}

// cbPublishOpen 熔断打开 → 写共享键（TTL 到期自动消失 = 半开）。
func (bl *Balancer) cbPublishOpen(id uint, until time.Time) {
	if bl.rdb == nil || bl.redisDown.get() {
		return
	}
	ttl := time.Until(until)
	if ttl <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := bl.rdb.Set(ctx, cbKey(id), until.UnixMilli(), ttl).Err(); err != nil {
		log.Printf("[slb] redis publish circuit open: %v (degrading)", err)
		bl.redisDown.set(true)
		go bl.watchRedis()
	}
}

// cbClear 供应商成功恢复 → 删除共享键。
func (bl *Balancer) cbClear(id uint) {
	if bl.rdb == nil || bl.redisDown.get() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := bl.rdb.Del(ctx, cbKey(id)).Err(); err != nil && err != redis.Nil {
		bl.redisDown.set(true)
		go bl.watchRedis()
	}
}

// healthy 判定供应商是否可用（调用方须已持有 bl.mu；本方法只取 b.mu）。
// 内存语义不变；分布式模式下以 ≤1/s 的节流读取共享打开状态。
func (bl *Balancer) healthy(b *breaker, id uint) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.Before(b.openUntil) {
		return false
	}
	if !b.openUntil.IsZero() { // 本地熔断到期 → 半开，恢复计数
		b.openUntil = time.Time{}
		b.fails = 0
	}
	if bl.rdb != nil && !bl.redisDown.get() && now.Sub(b.remoteCheckedAt) >= time.Second {
		b.remoteCheckedAt = now
		b.remoteOpenUntilMS = bl.remoteOpen(id)
	}
	if b.remoteOpenUntilMS > 0 {
		if now.UnixMilli() < b.remoteOpenUntilMS {
			return false // 任一实例打开的熔断在此同步生效
		}
		// 共享熔断到期 → 半开：本地计数清零，等待真实流量验证
		b.remoteOpenUntilMS = 0
		b.fails = 0
	}
	return true
}

// breakerLocked 返回（或创建）指定 provider 的熔断器；调用方须持有 bl.mu。
func (bl *Balancer) breakerLocked(id uint) *breaker {
	b, ok := bl.breakers[id]
	if !ok {
		b = &breaker{}
		bl.breakers[id] = b
	}
	return b
}

// reconcileCursor 对齐游标到当前别名上游集合：补建缺失项、剔除已移除项（配置热加载后集合变化）。
// 调用方须持有 bl.mu。
func (bl *Balancer) reconcileCursor(alias string, ups []runtime.ResolvedUpstream) map[uint]int {
	cw, ok := bl.rr[alias]
	if !ok {
		cw = map[uint]int{}
		bl.rr[alias] = cw
	}
	inSet := make(map[uint]bool, len(ups))
	for _, u := range ups {
		inSet[u.ProviderID] = true
	}
	for pid := range cw {
		if !inSet[pid] {
			delete(cw, pid)
		}
	}
	return cw
}

// Pick 按平滑加权轮询选取一个未尝试且未熔断的上游；全部不可用时返回 false。
// M-21：全局互斥只包裹纯内存操作——Redis 游标推进（最多 300ms RTT）挪到锁外，
// 否则一次网络抖动会把所有经网关请求的选路串行化。
func (bl *Balancer) Pick(alias string, ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	bl.mu.Lock()
	cands, total := bl.selectCandidatesLocked(alias, ups, tried, true)
	useRemote := len(tried) == 0 && bl.rdb != nil && !bl.redisDown.get()
	bl.mu.Unlock()

	if total <= 0 || len(cands) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	if useRemote {
		if picked, ok := bl.swrrRemote(alias, cands); ok {
			return picked, true
		}
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return swrrPick(bl.reconcileCursor(alias, ups), cands, total), true
}

// selectCandidatesLocked 健康/试选过滤 + 权重和；调用方须持有 bl.mu。
func (bl *Balancer) selectCandidatesLocked(alias string, ups []runtime.ResolvedUpstream, tried map[uint]bool, respectBreaker bool) ([]runtime.ResolvedUpstream, int) {
	bl.reconcileCursor(alias, ups)
	var candidates []runtime.ResolvedUpstream
	total := 0
	for _, u := range ups {
		if tried[u.ProviderID] {
			continue
		}
		if respectBreaker && !bl.healthy(bl.breakerLocked(u.ProviderID), u.ProviderID) {
			continue
		}
		candidates = append(candidates, u)
		total += u.Weight
	}
	return candidates, total
}

// PickIgnoringCircuit 所有上游都在熔断时兜底使用（可用性优先），仍按 SWRR 轮询。
func (bl *Balancer) PickIgnoringCircuit(alias string, ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	bl.mu.Lock()
	cands, total := bl.selectCandidatesLocked(alias, ups, tried, false)
	useRemote := len(tried) == 0 && bl.rdb != nil && !bl.redisDown.get()
	bl.mu.Unlock()

	if total <= 0 || len(cands) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	if useRemote {
		if picked, ok := bl.swrrRemote(alias, cands); ok {
			return picked, true
		}
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return swrrPick(bl.reconcileCursor(alias, ups), cands, total), true
}

// swrrPick 执行一步平滑加权轮询：每个候选 currentWeight += weight，
// 取 currentWeight 最大者（并列取切片首个，保证确定性），再 currentWeight[选中] -= total。
func swrrPick(cw map[uint]int, candidates []runtime.ResolvedUpstream, total int) runtime.ResolvedUpstream {
	var best runtime.ResolvedUpstream
	bestCW := 0
	first := true
	for _, u := range candidates {
		ncw := cw[u.ProviderID] + u.Weight
		cw[u.ProviderID] = ncw
		if first || ncw > bestCW {
			bestCW = ncw
			best = u
			first = false
		}
	}
	cw[best.ProviderID] -= total
	return best
}

// br 返回（或创建）指定 provider 的熔断器（线程安全，自取 bl.mu）。
func (bl *Balancer) br(id uint) *breaker {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return bl.breakerLocked(id)
}

// RecordSuccess / RecordFailure 维护熔断计数。
// 分布式模式：成功清除共享打开键；打开事件发布共享键（TTL 自动半开）。
func (bl *Balancer) RecordSuccess(id uint) {
	b := bl.br(id)
	b.mu.Lock()
	b.fails = 0
	b.openUntil = time.Time{}
	b.remoteOpenUntilMS = 0
	b.lastErr = ""
	b.mu.Unlock()
	bl.cbClear(id)
}

func (bl *Balancer) RecordFailure(id uint, fo settings.Failover, errMsg string) {
	var openUntil time.Time
	b := bl.br(id)
	b.mu.Lock()
	b.fails++
	b.lastErr = errMsg
	b.lastFail = time.Now()
	if fo.Enabled && b.fails >= fo.CircuitFailureThreshold {
		b.openUntil = time.Now().Add(time.Duration(fo.CircuitResetSeconds) * time.Second)
		openUntil = b.openUntil
	}
	b.mu.Unlock()
	if !openUntil.IsZero() { // 熔断打开 → 广播到 Redis（TTL=重置秒数，到期自动半开）
		bl.cbPublishOpen(id, openUntil)
	}
}

type Health struct {
	ProviderID uint   `json:"provider_id"`
	Provider   string `json:"provider"`
	Protocol   string `json:"protocol"`
	Healthy    bool   `json:"healthy"`
	FailCount  int    `json:"fail_count"`
	LastError  string `json:"last_error"`
}

func (bl *Balancer) HealthList(snap *runtime.Snapshot) []Health {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	var out []Health
	for id, p := range snap.Providers {
		h := Health{ProviderID: id, Provider: p.Name, Protocol: p.Protocol, Healthy: true}
		if b, ok := bl.breakers[id]; ok {
			h.Healthy = bl.healthy(b, id)
			b.mu.Lock()
			h.FailCount = b.fails
			h.LastError = b.lastErr
			b.mu.Unlock()
		} else if openMS := bl.remoteOpen(id); openMS > 0 && time.Now().UnixMilli() < openMS {
			// 本实例还没有该供应商的熔断器：直接读共享状态，
			// 让「他实例已打开」在运行状态页也如实反映（管理页低频，无需节流）。
			h.Healthy = false
		}
		out = append(out, h)
	}
	return out
}
