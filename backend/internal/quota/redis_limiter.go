// Package quota — redis_limiter.go 提供基于 Redis 的分布式固定窗口限流。
//
// 设计（多节点 REQ，第九轮后续）：
//   - 每个 (ruleID, apiKeyID, alias) 维度一个窗口键，TTL=窗口秒数；
//   - INCR + 首次 EXPIRE（原子 Lua）：竞态安全，N 实例共享同一计数；
//   - Redis 不可用时降级到本地内存窗口（可用性优先，退化为单实例语义并打印告警）；
//   - 与内存版 Allow 相同的匹配语义（APIKeyID=0 全局、ModelAlias=* 全部）。
package quota

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
)

// RedisLimiter 分布式固定窗口限流器。
type RedisLimiter struct {
	rdb      *redis.Client
	degraded atomicBool // Redis 探活失败时置位，走本地降级
	local    *RateLimiter
}

type atomicBool struct{ v int32 }

func (b *atomicBool) set(v bool) {
	if v {
		b.v = 1
	} else {
		b.v = 0
	}
}
func (b *atomicBool) get() bool { return b.v == 1 }

// NewRedisLimiter 用给定 Redis 地址创建分布式限流器（立即探活一次）。
func NewRedisLimiter(addr, password string) *RedisLimiter {
	rl := &RedisLimiter{
		rdb: redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     password,
			DialTimeout:  500 * time.Millisecond,
			ReadTimeout:  300 * time.Millisecond,
			WriteTimeout: 300 * time.Millisecond,
			PoolSize:     20,
		}),
		local: NewRateLimiter(nil),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rl.rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[ratelimit] WARNING: redis %s unreachable (%v); starting DEGRADED (per-instance) until it recovers", addr, err)
		rl.degraded.set(true)
	} else {
		log.Printf("[ratelimit] redis %s connected: distributed rate limiting ENABLED", addr)
	}
	return rl
}

// luaIncrWindow 原子 INCR + 首次 EXPIRE：
// 返回自增后的计数；TTL 只在计数=1（窗口首个请求）时设置，避免续期导致窗口漂移。
var luaIncrWindow = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// windowKey 限流窗口键。前缀带版本号，改语义时换前缀避免旧键污染。
func windowKey(ruleID, apiKeyID uint, alias string) string {
	return fmt.Sprintf("gw:rl:v1:%d:%d:%s", ruleID, apiKeyID, alias)
}

// Allow 依据快照中的限流规则判断是否放行（分布式计数）。
// 匹配语义与 RateLimiter.Allow 完全一致；Redis 故障时降级本地。
func (rl *RedisLimiter) Allow(snap *runtime.Snapshot, apiKeyID uint, alias string) (bool, *model.RateLimitRule) {
	if rl.degraded.get() {
		return rl.local.Allow(snap, apiKeyID, alias)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	for i := range snap.RateLimits {
		r := &snap.RateLimits[i]
		if r.APIKeyID != 0 && r.APIKeyID != apiKeyID {
			continue
		}
		if r.ModelAlias != "" && r.ModelAlias != "*" && r.ModelAlias != alias {
			continue
		}
		key := windowKey(r.ID, apiKeyID, alias)
		ttl := time.Duration(max1(r.WindowSeconds)) * time.Second
		n, err := luaIncrWindow.Run(ctx, rl.rdb, []string{key}, int64(ttl/time.Millisecond)).Int64()
		if err != nil {
			// Redis 故障：本次放行计数降级本地，置降级标记避免每请求都打超时
			log.Printf("[ratelimit] redis error (%v); DEGRADED to per-instance for this window", err)
			rl.degraded.set(true)
			go rl.watchRecovery()
			return rl.local.Allow(snap, apiKeyID, alias)
		}
		if n > int64(max1(r.MaxRequests)) {
			return false, r
		}
	}
	return true, nil
}

// watchRecovery 降级期间每 5s 探活 Redis，恢复后切回分布式计数。
func (rl *RedisLimiter) watchRecovery() {
	for {
		time.Sleep(5 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := rl.rdb.Ping(ctx).Err()
		cancel()
		if err == nil {
			rl.degraded.set(false)
			log.Printf("[ratelimit] redis recovered: distributed rate limiting re-enabled")
			return
		}
	}
}

// FlushHits 分布式模式下拒绝计数直接以 Redis 判定结果在网关侧累计，无需落库合并；
// 保留空实现以满足与 RateLimiter 相同的调用形态（如需要每规则统计可扩展）。
func (rl *RedisLimiter) FlushHits() {}
