package slb

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"llmrouter/internal/runtime"
)

// startRedis 为集成测试拉起临时 redis-server（无认证、随机未占用端口由其自身选择失败时跳过）。
// 优先复用 GW_TEST_REDIS_ADDR 指向的实例；都没有则跳过（CI 里由 services redis 提供）。
func startRedis(t *testing.T) string {
	t.Helper()
	if addr := os.Getenv("GW_TEST_REDIS_ADDR"); addr != "" {
		return addr
	}
	if _, err := exec.LookPath("redis-server"); err != nil {
		t.Skip("redis-server not installed and GW_TEST_REDIS_ADDR unset")
	}
	// 端口竞态可接受：测试机独占场景
	port := 16399
	cmd := exec.Command("redis-server", "--port", fmt.Sprint(port),
		"--save", "", "--appendonly", "no", "--daemonize", "no")
	if err := cmd.Start(); err != nil {
		t.Skipf("start redis-server: %v", err)
	}
	// 等就绪
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		bl := New()
		bl.SetRedis(addr, "")
		if !bl.redisDown.get() {
			t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
			return addr
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	t.Skip("redis-server did not become ready in 3s")
	return ""
}

func mkUps(ids ...int) []runtime.ResolvedUpstream {
	ups := make([]runtime.ResolvedUpstream, 0, len(ids))
	for _, id := range ids {
		ups = append(ups, runtime.ResolvedUpstream{
			ProviderID: uint(id), ProviderName: fmt.Sprintf("p%d", id), Protocol: "openai_chat", Weight: 1,
		})
	}
	return ups
}

// TestSwrrRemote_GlobalCursorDistribution 两个"实例"（两个 Balancer）对同一别名
// 交替 Pick，全局游标保证严格 1:1 交替（A,B,A,B…），且两实例各自序列互补不重叠。
func TestSwrrRemote_GlobalCursorDistribution(t *testing.T) {
	addr := startRedis(t)
	ups := mkUps(101, 102)

	bl1, bl2 := New(), New()
	bl1.SetRedis(addr, "")
	bl2.SetRedis(addr, "")
	if bl1.redisDown.get() || bl2.redisDown.get() {
		t.Fatal("redis should be up")
	}

	got := make([]uint, 0, 8)
	for i := 0; i < 8; i++ {
		bl := bl1
		if i%2 == 1 {
			bl = bl2
		}
		u, ok := bl.Pick("alias-x", ups, nil)
		if !ok {
			t.Fatalf("pick %d failed", i)
		}
		got = append(got, u.ProviderID)
	}
	// 严格交替：偶数位=101，奇数位=102
	for i, id := range got {
		want := uint(101)
		if i%2 == 1 {
			want = 102
		}
		if id != want {
			t.Fatalf("global cursor sequence broken at %d: got=%v", i, got)
		}
	}
}

// TestSwrrRemote_WeightRatio 权重 2:1 → 全局游标 6 连发严格按 2/3、1/3 配额轮转。
func TestSwrrRemote_WeightRatio(t *testing.T) {
	addr := startRedis(t)
	ups := []runtime.ResolvedUpstream{
		{ProviderID: 201, Protocol: "openai_chat", Weight: 2},
		{ProviderID: 202, Protocol: "openai_chat", Weight: 1},
	}
	bl := New()
	bl.SetRedis(addr, "")
	if bl.redisDown.get() {
		t.Fatal("redis should be up")
	}
	counts := map[uint]int{}
	for i := 0; i < 6; i++ {
		u, ok := bl.Pick("alias-w", ups, nil)
		if !ok {
			t.Fatal("pick failed")
		}
		counts[u.ProviderID]++
	}
	if counts[201] != 4 || counts[202] != 2 {
		t.Fatalf("weight 2:1 over 6 picks: got %v, want 4/2", counts)
	}
}

// TestSwrrRemote_DegradesToLocal 当 Redis 掉线（指向不存在端口）时自动降级
// 本地游标：Pick 仍成功返回（不阻塞热路径），redisDown 置位。
func TestSwrrRemote_DegradesToLocal(t *testing.T) {
	bl := New()
	bl.SetRedis("127.0.0.1:1", "") // 必然连不上
	if !bl.redisDown.get() {
		t.Fatal("expected redisDown")
	}
	ups := mkUps(301, 302)
	for i := 0; i < 4; i++ {
		u, ok := bl.Pick("alias-d", ups, nil)
		if !ok {
			t.Fatalf("pick %d should fall back to local cursor", i)
		}
		if u.ProviderID != 301 && u.ProviderID != 302 {
			t.Fatalf("unexpected pick %v", u.ProviderID)
		}
	}
}

// TestSwrrRemote_TriedBypassesRemote 故障转移重试路径（tried 非空）不使用全局游标，
// 走本地游标（候选动态、无法跨步重放全局序列）。
func TestSwrrRemote_TriedBypassesRemote(t *testing.T) {
	addr := startRedis(t)
	ups := mkUps(401, 402)
	bl := New()
	bl.SetRedis(addr, "")
	if bl.redisDown.get() {
		t.Fatal("redis should be up")
	}
	u, ok := bl.Pick("alias-t", ups, map[uint]bool{401: true})
	if !ok || u.ProviderID != 402 {
		t.Fatalf("tried-filter pick failed: ok=%v id=%v", ok, u.ProviderID)
	}
}
