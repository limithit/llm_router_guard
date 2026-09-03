// Package slb 实现加权轮询负载均衡与熔断（REQ-006/007）。
// 熔断状态按供应商维度维护于内存（单实例语义），打开期间选择器自动跳过。
// 负载策略为平滑加权轮询（SWRR, smooth weighted round-robin）：按别名维护
// currentWeight 游标，确定性分发、不聚堆，比加权随机更可预期。
package slb

import (
	"sync"
	"time"

	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
)

type breaker struct {
	mu        sync.Mutex
	fails     int
	openUntil time.Time
	lastErr   string
	lastFail  time.Time
}

func (b *breaker) healthy() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.openUntil) {
		return false
	}
	if !b.openUntil.IsZero() { // 熔断到期 → 半开，恢复计数
		b.openUntil = time.Time{}
		b.fails = 0
	}
	return true
}

// Balancer 从别名上游集合中按权重轮询挑选供应商，支持故障转移时排除已尝试项。
type Balancer struct {
	mu       sync.Mutex
	breakers map[uint]*breaker
	// rr: 别名 → (providerID → currentWeight) 游标，SWRR 状态。
	rr map[string]map[uint]int
}

func New() *Balancer {
	return &Balancer{breakers: map[uint]*breaker{}, rr: map[string]map[uint]int{}}
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

// br 返回（或创建）指定 provider 的熔断器（线程安全，自取 bl.mu）。
func (bl *Balancer) br(id uint) *breaker {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return bl.breakerLocked(id)
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
func (bl *Balancer) Pick(alias string, ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	cw := bl.reconcileCursor(alias, ups)

	var candidates []runtime.ResolvedUpstream
	total := 0
	for _, u := range ups {
		if tried[u.ProviderID] {
			continue
		}
		if !bl.breakerLocked(u.ProviderID).healthy() {
			continue
		}
		candidates = append(candidates, u)
		total += u.Weight
	}
	if total <= 0 || len(candidates) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	return swrrPick(cw, candidates, total), true
}

// PickIgnoringCircuit 所有上游都在熔断时兜底使用（可用性优先），仍按 SWRR 轮询。
func (bl *Balancer) PickIgnoringCircuit(alias string, ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	cw := bl.reconcileCursor(alias, ups)

	var candidates []runtime.ResolvedUpstream
	total := 0
	for _, u := range ups {
		if tried[u.ProviderID] {
			continue
		}
		candidates = append(candidates, u)
		total += u.Weight
	}
	if total <= 0 || len(candidates) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	return swrrPick(cw, candidates, total), true
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

// RecordSuccess / RecordFailure 维护熔断计数。
func (bl *Balancer) RecordSuccess(id uint) {
	b := bl.br(id)
	b.mu.Lock()
	b.fails = 0
	b.openUntil = time.Time{}
	b.lastErr = ""
	b.mu.Unlock()
}

func (bl *Balancer) RecordFailure(id uint, fo settings.Failover, errMsg string) {
	b := bl.br(id)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fails++
	b.lastErr = errMsg
	b.lastFail = time.Now()
	if fo.Enabled && b.fails >= fo.CircuitFailureThreshold {
		b.openUntil = time.Now().Add(time.Duration(fo.CircuitResetSeconds) * time.Second)
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
		b, ok := bl.breakers[id]
		h := Health{ProviderID: id, Provider: p.Name, Protocol: p.Protocol, Healthy: true}
		if ok {
			b.mu.Lock()
			h.Healthy = time.Now().After(b.openUntil)
			h.FailCount = b.fails
			h.LastError = b.lastErr
			b.mu.Unlock()
		}
		out = append(out, h)
	}
	return out
}
