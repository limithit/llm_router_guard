package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"llmrouter/internal/crypto"
	"llmrouter/internal/db"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
)

func newChecker(t *testing.T) (*Checker, *slb.Balancer) {
	t.Helper()
	gdb, err := db.Open("sqlite", filepath.Join(t.TempDir(), "health_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := gdb.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	enc, err := crypto.NewCipher("test-master-key")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	bl := slb.New()
	return New(gdb, enc, bl), bl
}

func seed(t *testing.T, c *Checker, name, baseURL string, enabled bool) uint {
	t.Helper()
	p := model.Provider{Name: name, Protocol: "openai_chat", BaseURL: baseURL, Enabled: enabled}
	if err := c.db.Create(&p).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	return p.ID
}

// snapWith 用给定供应商构造最小快照（HealthList 遍历 snap.Providers，nil 会 panic）。
func snapWith(ids ...uint) *runtime.Snapshot {
	snap := &runtime.Snapshot{Providers: map[uint]*model.Provider{}}
	for _, id := range ids {
		snap.Providers[id] = &model.Provider{ID: id, Name: "p", Protocol: "openai_chat"}
	}
	return snap
}

// TestProbeAll_ReachesEndpointAndClearsCircuit 端点可达（哪怕 401）→ RecordSuccess 清熔断。
func TestProbeAll_ReachesEndpointAndClearsCircuit(t *testing.T) {
	c, bl := newChecker(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("probe path = %q, want /v1/models", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized) // 可达即健康：认证错误不算传输故障
	}))
	defer srv.Close()
	id := seed(t, c, "p-ok", srv.URL, true)

	// 预置：本地熔断打开（模拟历史故障），探测成功后应清零恢复
	fo := settings.DefaultFailover()
	bl.RecordFailure(id, fo, "pre-existing failure")
	bl.RecordFailure(id, fo, "pre-existing failure")

	c.ProbeAll(context.Background())

	found := false
	for _, h := range bl.HealthList(snapWith(id)) {
		if h.ProviderID == id {
			found = true
			if !h.Healthy {
				t.Fatalf("provider should be healthy after successful probe")
			}
			if h.FailCount != 0 {
				t.Fatalf("fail_count = %d, want 0 after success", h.FailCount)
			}
		}
	}
	if !found {
		t.Fatal("provider missing from health list")
	}
}

// TestProbeAll_TransportFailureAccumulates 传输层失败 → 计数累计到阈值即打开熔断。
func TestProbeAll_TransportFailureAccumulates(t *testing.T) {
	c, bl := newChecker(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	down := srv.URL
	srv.Close() // 制造连接拒绝

	id := seed(t, c, "p-down", down, true)
	fo := settings.DefaultFailover() // threshold=5
	for _, h := range bl.HealthList(snapWith(id)) {
		if h.ProviderID == id && !h.Healthy {
			t.Fatal("precondition: should start healthy")
		}
	}
	// 4 次：未到阈值，仍健康
	for i := 0; i < fo.CircuitFailureThreshold-1; i++ {
		c.ProbeAll(context.Background())
	}
	for _, h := range bl.HealthList(snapWith(id)) {
		if h.ProviderID == id && !h.Healthy {
			t.Fatalf("below threshold should stay healthy (fails=%d)", h.FailCount)
		}
	}
	// 第 5 次：达到阈值，打开
	c.ProbeAll(context.Background())
	opened := false
	for _, h := range bl.HealthList(snapWith(id)) {
		if h.ProviderID == id && !h.Healthy {
			opened = true
		}
	}
	if !opened {
		t.Fatal("reaching threshold should open the circuit")
	}
}

// TestProbeAll_DisabledProviderSkipped 禁用供应商不探测。
func TestProbeAll_DisabledProviderSkipped(t *testing.T) {
	c, _ := newChecker(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("disabled provider must not be probed")
	}))
	defer srv.Close()
	seed(t, c, "p-disabled", srv.URL, false)
	c.ProbeAll(context.Background()) // 不触发 t.Error 即通过
}

// TestRun_DisabledWhenIntervalZero interval<=0 时 Run 必须直接返回（可安全 go 出去）。
func TestRun_DisabledWhenIntervalZero(t *testing.T) {
	c, _ := newChecker(t)
	done := make(chan struct{})
	go func() {
		c.Run(context.Background(), 0)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run should return immediately when interval <= 0")
	}
}
