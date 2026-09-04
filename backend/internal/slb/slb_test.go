package slb

import (
	"testing"
	"time"

	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
)

// ---- Helper: 构建测试用上游列表 ----

func makeUpstreams() []runtime.ResolvedUpstream {
	return []runtime.ResolvedUpstream{
		{ProviderID: 1, ProviderName: "openai-1", Protocol: model.ProtoOpenAIChat, UpstreamModel: "gpt-4o", Weight: 5},
		{ProviderID: 2, ProviderName: "openai-2", Protocol: model.ProtoOpenAIChat, UpstreamModel: "gpt-4o", Weight: 3},
		{ProviderID: 3, ProviderName: "anthropic-1", Protocol: model.ProtoAnthropic, UpstreamModel: "claude-3", Weight: 2},
	}
}

func makeSnap() *runtime.Snapshot {
	return &runtime.Snapshot{
		Version: "test",
		Counts:  map[string]int{},
		Providers: map[uint]*model.Provider{
			1: {ID: 1, Name: "openai-1", Protocol: model.ProtoOpenAIChat},
			2: {ID: 2, Name: "openai-2", Protocol: model.ProtoOpenAIChat},
			3: {ID: 3, Name: "anthropic-1", Protocol: model.ProtoAnthropic},
		},
		ProviderByName: map[string]*model.Provider{},
		Aliases:        map[string][]runtime.ResolvedUpstream{},
		APIKeys:        map[string]*model.APIKey{},
	}
}

func defaultFailover() settings.Failover {
	return settings.Failover{
		Enabled:                 true,
		RetryCount:              2,
		Backoff:                 "exponential",
		RetryIntervalMs:         200,
		TriggerStatusCodes:      []int{408, 429, 500, 502, 503, 504},
		CircuitFailureThreshold: 3,
		CircuitResetSeconds:     2, // 短时间方便测试
	}
}

// ---- Pick 基本功能测试 ----

func TestPick_BasicSelection(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	picked, ok := bl.Pick("a", ups, nil)
	if !ok {
		t.Fatal("Pick should succeed with available upstreams")
	}
	if picked.ProviderID == 0 {
		t.Error("Picked upstream should have non-zero ProviderID")
	}
}

func TestPick_RespectsTried(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	// Mark provider 1 as tried
	tried := map[uint]bool{1: true}
	picked, ok := bl.Pick("a", ups, tried)
	if !ok {
		t.Fatal("Pick should succeed with remaining upstreams")
	}
	if picked.ProviderID == 1 {
		t.Error("Pick should not return tried provider 1")
	}
}

func TestPick_AllTried(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	// All providers tried
	tried := map[uint]bool{1: true, 2: true, 3: true}
	_, ok := bl.Pick("a", ups, tried)
	if ok {
		t.Error("Pick should fail when all providers are tried")
	}
}

func TestPick_EmptyUpstreams(t *testing.T) {
	bl := New()

	_, ok := bl.Pick("a", nil, nil)
	if ok {
		t.Error("Pick should fail with empty upstream list")
	}
}

func TestPick_ZeroWeight(t *testing.T) {
	bl := New()
	ups := []runtime.ResolvedUpstream{
		{ProviderID: 1, ProviderName: "zero-weight", Weight: 0},
	}

	_, ok := bl.Pick("a", ups, nil)
	if ok {
		t.Error("Pick should fail when all upstreams have zero weight")
	}
}

func TestPick_NilTried(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	// nil tried map should work (treated as empty)
	_, ok := bl.Pick("a", ups, nil)
	if !ok {
		t.Error("Pick should work with nil tried map")
	}
}

func TestPick_SingleUpstream(t *testing.T) {
	bl := New()
	ups := []runtime.ResolvedUpstream{
		{ProviderID: 1, ProviderName: "only-one", Weight: 1},
	}

	picked, ok := bl.Pick("a", ups, nil)
	if !ok {
		t.Fatal("Pick should succeed with single upstream")
	}
	if picked.ProviderID != 1 {
		t.Errorf("Picked ProviderID = %d, want 1", picked.ProviderID)
	}
}

// ---- 权重分布测试 ----

func TestPick_WeightDistribution(t *testing.T) {
	bl := New()
	ups := makeUpstreams() // weights: 5, 3, 2 (total 10)

	counts := map[uint]int{}
	iterations := 10000

	for i := 0; i < iterations; i++ {
		picked, ok := bl.Pick("a", ups, nil)
		if !ok {
			t.Fatal("Pick failed during distribution test")
		}
		counts[picked.ProviderID]++
	}

	// SWRR 是确定性加权轮询：每 total(=10) 次恰好按权重比例分发，
	// 故 10000 次 = 1000 轮，应严格等于 5000/3000/2000。
	if counts[1] != 5000 {
		t.Errorf("provider 1 (weight 5): got %d, want 5000", counts[1])
	}
	if counts[2] != 3000 {
		t.Errorf("provider 2 (weight 3): got %d, want 3000", counts[2])
	}
	if counts[3] != 2000 {
		t.Errorf("provider 3 (weight 2): got %d, want 2000", counts[3])
	}
}

// ---- 熔断器测试 ----

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Record failures below threshold
	for i := 0; i < fo.CircuitFailureThreshold-1; i++ {
		bl.RecordFailure(1, fo, "timeout")
	}

	// Provider 1 should still be selectable (circuit not yet open)
	picked, ok := bl.Pick("a", ups, nil)
	if !ok {
		t.Fatal("Pick should succeed before circuit opens")
	}
	_ = picked

	// Record one more failure to hit threshold
	bl.RecordFailure(1, fo, "timeout")

	// Now provider 1 should be circuit-broken
	// Run multiple picks to verify provider 1 is never selected
	for i := 0; i < 100; i++ {
		picked, ok := bl.Pick("a", ups, nil)
		if !ok {
			t.Fatal("Pick should succeed with other providers available")
		}
		if picked.ProviderID == 1 {
			t.Error("Provider 1 should be circuit-broken and not selected")
		}
	}
}

func TestCircuitBreaker_ResetsAfterTimeout(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	fo.CircuitResetSeconds = 0 // Reset immediately (for testing)

	// Open the circuit
	for i := 0; i < fo.CircuitFailureThreshold; i++ {
		bl.RecordFailure(1, fo, "timeout")
	}

	// Wait a tiny bit for the circuit to reset
	time.Sleep(10 * time.Millisecond)

	// Provider 1 should be available again (circuit reset)
	b := bl.br(1)
	if !bl.healthy(b, 1) {
		t.Error("Circuit should have reset after timeout")
	}
}

func TestCircuitBreaker_RecordSuccessClears(t *testing.T) {
	bl := New()
	fo := defaultFailover()

	// Record some failures (below threshold)
	bl.RecordFailure(1, fo, "err1")
	bl.RecordFailure(1, fo, "err2")

	// Record success - should clear failures
	bl.RecordSuccess(1)

	b := bl.br(1)
	b.mu.Lock()
	fails := b.fails
	openUntil := b.openUntil
	b.mu.Unlock()

	if fails != 0 {
		t.Errorf("After success, fails should be 0, got %d", fails)
	}
	if !openUntil.IsZero() {
		t.Error("After success, openUntil should be zero")
	}
}

func TestCircuitBreaker_DoesNotOpenWhenDisabled(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	fo.Enabled = false

	// Record many failures - should NOT open circuit
	for i := 0; i < 20; i++ {
		bl.RecordFailure(1, fo, "timeout")
	}

	b := bl.br(1)
	b.mu.Lock()
	openUntil := b.openUntil
	fails := b.fails
	b.mu.Unlock()

	if !openUntil.IsZero() {
		t.Error("Circuit should not open when failover is disabled")
	}
	if fails != 20 {
		t.Errorf("Fail count should be 20, got %d", fails)
	}
}

func TestCircuitBreaker_PartialAvailability(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Open circuit for provider 1
	for i := 0; i < fo.CircuitFailureThreshold; i++ {
		bl.RecordFailure(1, fo, "timeout")
	}

	// Pick should still work, just skip provider 1
	picked, ok := bl.Pick("a", ups, nil)
	if !ok {
		t.Fatal("Pick should succeed with partial availability")
	}
	if picked.ProviderID == 1 {
		t.Error("Should skip circuit-broken provider 1")
	}

	// Open circuit for provider 2
	for i := 0; i < fo.CircuitFailureThreshold; i++ {
		bl.RecordFailure(2, fo, "timeout")
	}

	// Still should work with provider 3
	picked, ok = bl.Pick("a", ups, nil)
	if !ok {
		t.Fatal("Pick should succeed with one provider left")
	}
	if picked.ProviderID != 3 {
		t.Errorf("Expected provider 3, got %d", picked.ProviderID)
	}
}

func TestCircuitBreaker_AllBroken(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Open circuit for all providers
	for _, id := range []uint{1, 2, 3} {
		for i := 0; i < fo.CircuitFailureThreshold; i++ {
			bl.RecordFailure(id, fo, "timeout")
		}
	}

	// Pick should fail when all are circuit-broken
	_, ok := bl.Pick("a", ups, nil)
	if ok {
		t.Error("Pick should fail when all providers are circuit-broken")
	}
}

// ---- PickIgnoringCircuit 测试 ----

func TestPickIgnoringCircuit_AllBroken(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Open circuit for all providers
	for _, id := range []uint{1, 2, 3} {
		for i := 0; i < fo.CircuitFailureThreshold; i++ {
			bl.RecordFailure(id, fo, "timeout")
		}
	}

	// PickIgnoringCircuit should still work (availability-first fallback)
	picked, ok := bl.PickIgnoringCircuit("a", ups, nil)
	if !ok {
		t.Error("PickIgnoringCircuit should succeed even when all are broken")
	}
	if picked.ProviderID == 0 {
		t.Error("Should return a valid upstream")
	}
}

func TestPickIgnoringCircuit_RespectsTried(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	tried := map[uint]bool{1: true, 2: true}
	picked, ok := bl.PickIgnoringCircuit("a", ups, tried)
	if !ok {
		t.Fatal("Should succeed with remaining provider")
	}
	if picked.ProviderID != 3 {
		t.Errorf("Expected provider 3, got %d", picked.ProviderID)
	}
}

func TestPickIgnoringCircuit_AllTried(t *testing.T) {
	bl := New()
	ups := makeUpstreams()

	tried := map[uint]bool{1: true, 2: true, 3: true}
	_, ok := bl.PickIgnoringCircuit("a", ups, tried)
	if ok {
		t.Error("Should fail when all providers are tried")
	}
}

// ---- Failover 循环模拟 ----

func TestFailover_FullRetryLoop(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Simulate: try provider 1 (fails), try provider 2 (fails), provider 3 succeeds
	tried := map[uint]bool{}

	// First pick
	p1, ok := bl.Pick("a", ups, tried)
	if !ok {
		t.Fatal("First pick should succeed")
	}
	tried[p1.ProviderID] = true
	bl.RecordFailure(p1.ProviderID, fo, "500 error")

	// Second pick (skip first)
	p2, ok := bl.Pick("a", ups, tried)
	if !ok {
		t.Fatal("Second pick should succeed")
	}
	if p2.ProviderID == p1.ProviderID {
		t.Error("Second pick should be different from first")
	}
	tried[p2.ProviderID] = true
	bl.RecordFailure(p2.ProviderID, fo, "timeout")

	// Third pick (skip first two)
	p3, ok := bl.Pick("a", ups, tried)
	if !ok {
		t.Fatal("Third pick should succeed")
	}
	if p3.ProviderID == p1.ProviderID || p3.ProviderID == p2.ProviderID {
		t.Error("Third pick should be different from first two")
	}

	// Simulate success on third try
	bl.RecordSuccess(p3.ProviderID)

	// Verify provider 3's breaker is clean
	b := bl.br(p3.ProviderID)
	b.mu.Lock()
	if b.fails != 0 || !b.openUntil.IsZero() {
		t.Error("Provider 3 should have clean breaker after success")
	}
	b.mu.Unlock()
}

// ---- HealthList 测试 ----

func TestHealthList_AllHealthy(t *testing.T) {
	bl := New()
	snap := makeSnap()

	health := bl.HealthList(snap)
	if len(health) != 3 {
		t.Errorf("Expected 3 health entries, got %d", len(health))
	}

	for _, h := range health {
		if !h.Healthy {
			t.Errorf("Provider %s should be healthy", h.Provider)
		}
		if h.FailCount != 0 {
			t.Errorf("Provider %s should have 0 failures, got %d", h.Provider, h.FailCount)
		}
	}
}

func TestHealthList_WithFailures(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	snap := makeSnap()

	// Record some failures for provider 1
	bl.RecordFailure(1, fo, "timeout")
	bl.RecordFailure(1, fo, "500 error")

	health := bl.HealthList(snap)

	var p1Health *Health
	for i := range health {
		if health[i].ProviderID == 1 {
			p1Health = &health[i]
			break
		}
	}

	if p1Health == nil {
		t.Fatal("Provider 1 health not found")
	}
	if p1Health.FailCount != 2 {
		t.Errorf("Expected 2 failures, got %d", p1Health.FailCount)
	}
	if p1Health.LastError == "" {
		t.Error("Expected non-empty last error")
	}
}

func TestHealthList_WithCircuitOpen(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	snap := makeSnap()

	// Open circuit for provider 1
	for i := 0; i < fo.CircuitFailureThreshold; i++ {
		bl.RecordFailure(1, fo, "timeout")
	}

	health := bl.HealthList(snap)

	for _, h := range health {
		if h.ProviderID == 1 {
			if h.Healthy {
				t.Error("Provider 1 should be unhealthy (circuit open)")
			}
		}
	}
}

// ---- 并发安全测试 ----

func TestConcurrent_PickAndRecord(t *testing.T) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	done := make(chan bool, 4)

	// Concurrent picker
	go func() {
		for i := 0; i < 1000; i++ {
			bl.Pick("a", ups, nil)
		}
		done <- true
	}()

	// Concurrent failure recorder
	go func() {
		for i := 0; i < 1000; i++ {
			bl.RecordFailure(1, fo, "err")
		}
		done <- true
	}()

	// Concurrent success recorder
	go func() {
		for i := 0; i < 1000; i++ {
			bl.RecordSuccess(1)
		}
		done <- true
	}()

	// Concurrent health lister
	go func() {
		snap := makeSnap()
		for i := 0; i < 1000; i++ {
			bl.HealthList(snap)
		}
		done <- true
	}()

	// Wait for all to complete (with timeout)
	for i := 0; i < 4; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Concurrent test timed out")
		}
	}
}

// ---- 基准测试 ----

func BenchmarkPick_SingleProvider(b *testing.B) {
	bl := New()
	ups := []runtime.ResolvedUpstream{
		{ProviderID: 1, Weight: 1},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Pick("a", ups, nil)
	}
}

func BenchmarkPick_ThreeProviders(b *testing.B) {
	bl := New()
	ups := makeUpstreams()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Pick("a", ups, nil)
	}
}

func BenchmarkPick_WithCircuitBreaker(b *testing.B) {
	bl := New()
	fo := defaultFailover()
	ups := makeUpstreams()

	// Open circuit for one provider
	for i := 0; i < fo.CircuitFailureThreshold; i++ {
		bl.RecordFailure(1, fo, "err")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Pick("a", ups, nil)
	}
}

func BenchmarkRecordFailure(b *testing.B) {
	bl := New()
	fo := defaultFailover()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.RecordFailure(1, fo, "err")
		bl.RecordSuccess(1) // reset to avoid circuit open
	}
}

func BenchmarkHealthList(b *testing.B) {
	bl := New()
	snap := makeSnap()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.HealthList(snap)
	}
}
