// Package slb 实现加权负载均衡与熔断（REQ-006/007）。
// 熔断状态按供应商维度维护于内存（单实例语义），打开期间选择器自动跳过。
package slb

import (
	"math/rand"
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

// Balancer 从别名上游集合中按权重挑选供应商，支持故障转移时排除已尝试项。
type Balancer struct {
	mu       sync.Mutex
	breakers map[uint]*breaker
}

func New() *Balancer { return &Balancer{breakers: map[uint]*breaker{}} }

func (bl *Balancer) br(id uint) *breaker {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	b, ok := bl.breakers[id]
	if !ok {
		b = &breaker{}
		bl.breakers[id] = b
	}
	return b
}

// Pick 按权重随机选取一个未尝试且未熔断的上游；全部不可用时返回 false。
func (bl *Balancer) Pick(ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	total := 0
	var candidates []runtime.ResolvedUpstream
	for _, u := range ups {
		if tried[u.ProviderID] {
			continue
		}
		if !bl.br(u.ProviderID).healthy() {
			continue
		}
		candidates = append(candidates, u)
		total += u.Weight
	}
	if total <= 0 || len(candidates) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	r := rand.Intn(total)
	for _, u := range candidates {
		r -= u.Weight
		if r < 0 {
			return u, true
		}
	}
	return candidates[len(candidates)-1], true
}

// PickIgnoringCircuit 所有上游都在熔断时兜底使用（可用性优先）。
func (bl *Balancer) PickIgnoringCircuit(ups []runtime.ResolvedUpstream, tried map[uint]bool) (runtime.ResolvedUpstream, bool) {
	total := 0
	var candidates []runtime.ResolvedUpstream
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
	r := rand.Intn(total)
	for _, u := range candidates {
		r -= u.Weight
		if r < 0 {
			return u, true
		}
	}
	return candidates[len(candidates)-1], true
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
