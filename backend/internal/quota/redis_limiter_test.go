// redis_limiter_test.go — 分布式限流器（Redis）单元测试。
// miniredis 内存 Redis：验证多实例共享计数、窗口过期恢复、Redis 故障降级本地。
package quota

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
)

func redisSnap(ruleID uint, max int) *runtime.Snapshot {
	return makeSnap([]model.RateLimitRule{
		{ID: ruleID, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: max, Enabled: true},
	}, nil)
}

func TestRedisLimiter_GlobalCountAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	snap := redisSnap(1, 3)

	// 模拟两个网关实例，共享同一个 Redis
	rlA := NewRedisLimiter(mr.Addr(), "")
	rlB := NewRedisLimiter(mr.Addr(), "")

	for i := 0; i < 3; i++ {
		rl := rlA
		if i%2 == 1 {
			rl = rlB // 交替经过两个实例
		}
		if ok, _ := rl.Allow(snap, 1, "gpt-4o"); !ok {
			t.Fatalf("request %d (instance %s) should be allowed", i+1, map[int]string{0: "A", 1: "B"}[i%2])
		}
	}
	// 第 4 次请求无论经过哪个实例都应被拒（全局共享计数）
	if ok, _ := rlB.Allow(snap, 1, "gpt-4o"); ok {
		t.Error("4th request via instance B should be blocked globally")
	}
	if ok, _ := rlA.Allow(snap, 1, "gpt-4o"); ok {
		t.Error("4th request via instance A should be blocked globally")
	}
	// 不同 apiKeyID 独立窗口
	if ok, _ := rlA.Allow(snap, 7, "gpt-4o"); !ok {
		t.Error("different apiKeyID should have its own window")
	}
	// 不同别名独立窗口
	if ok, _ := rlB.Allow(snap, 1, "other-alias"); !ok {
		t.Error("different alias should have its own window")
	}
}

func TestRedisLimiter_WindowExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 2, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 10, MaxRequests: 1, Enabled: true},
	}, nil)
	rl := NewRedisLimiter(mr.Addr(), "")

	if ok, _ := rl.Allow(snap, 3, "m"); !ok {
		t.Fatal("first request should pass")
	}
	if ok, _ := rl.Allow(snap, 3, "m"); ok {
		t.Fatal("second request within window should be blocked")
	}
	// 快进模拟窗口过期：TTL 到期后计数应重新开始
	mr.FastForward(11 * time.Second)
	if ok, _ := rl.Allow(snap, 3, "m"); !ok {
		t.Fatal("after window expiry the request should pass again")
	}
}

func TestRedisLimiter_DegradedToLocalWhenRedisDown(t *testing.T) {
	// 端口 1 无人监听：启动即探活失败 → 降级本地内存限流（仍有效，只是单实例语义）
	rl := NewRedisLimiter("127.0.0.1:1", "")
	snap := redisSnap(9, 2)

	for i := 0; i < 2; i++ {
		if ok, _ := rl.Allow(snap, 1, "gpt-4o"); !ok {
			t.Fatalf("degraded request %d should be allowed", i+1)
		}
	}
	if ok, _ := rl.Allow(snap, 1, "gpt-4o"); ok {
		t.Error("degraded 3rd request should still be blocked by the local window")
	}
}
