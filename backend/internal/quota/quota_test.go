package quota

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/db"
	"llmrouter/internal/model"
	"llmrouter/internal/runtime"
	"llmrouter/internal/settings"
)

// ---- Helper ----

func makeSnap(rls []model.RateLimitRule, quotas []model.Quota) *runtime.Snapshot {
	return &runtime.Snapshot{
		Version:        "test",
		Counts:         map[string]int{},
		Providers:      map[uint]*model.Provider{},
		ProviderByName: map[string]*model.Provider{},
		Aliases:        map[string][]runtime.ResolvedUpstream{},
		APIKeys:        map[string]*model.APIKey{},
		RateLimits:     rls,
		Quotas:         quotas,
		General:        settings.DefaultGeneral(),
		Output:         settings.DefaultOutputFilter(),
		Failover:       settings.DefaultFailover(),
		Security:       settings.DefaultSecurity(),
		Alerts:         settings.DefaultQuotaAlerts(),
	}
}

func mustOpenDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return gdb
}

// closeDB 关闭 gorm.DB 底层 *sql.DB（*gorm.DB 本身无 Close 方法）。
func closeDB(gdb *gorm.DB) {
	if sqlDB, err := gdb.DB(); err == nil {
		sqlDB.Close()
	}
}

// ========================
// NextReset 纯函数测试
// ========================

func TestNextReset_Day(t *testing.T) {
	now := time.Date(2026, 9, 2, 15, 30, 0, 0, time.UTC)
	reset := NextReset("day", now)

	expected := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(day) = %v, want %v", reset, expected)
	}
}

func TestNextReset_Week(t *testing.T) {
	// 2026-09-02 is a Wednesday
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	reset := NextReset("week", now)

	// Next Monday = 2026-09-07
	expected := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(week) = %v, want %v", reset, expected)
	}
}

func TestNextReset_WeekOnMonday(t *testing.T) {
	// 2026-09-07 is a Monday
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	reset := NextReset("week", now)

	// Should be next Monday = 2026-09-14（当天为周一时，下次重置为 +7 天后）
	expected := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(week, Monday) = %v, want %v", reset, expected)
	}
}

func TestNextReset_Month(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	reset := NextReset("month", now)

	expected := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(month) = %v, want %v", reset, expected)
	}
}

func TestNextReset_MonthYearRollover(t *testing.T) {
	now := time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)
	reset := NextReset("month", now)

	expected := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(month, Dec 31) = %v, want %v", reset, expected)
	}
}

func TestNextReset_UnknownPeriod(t *testing.T) {
	now := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	reset := NextReset("unknown", now)

	// Should default to day
	expected := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(unknown) = %v, want %v (day default)", reset, expected)
	}
}

func TestNextReset_Midnight(t *testing.T) {
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	reset := NextReset("day", now)

	expected := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	if !reset.Equal(expected) {
		t.Errorf("NextReset(day, midnight) = %v, want %v", reset, expected)
	}
}

// ========================
// matchQuotas 测试
// ========================

func TestMatchQuotas_ExactAlias(t *testing.T) {
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
		{ID: 2, APIKeyID: 10, ModelAlias: "claude-3", QuotaType: "requests", Period: "day", LimitValue: 50, Enabled: true},
	})

	matched := matchQuotas(snap, 10, "gpt-4o")
	if len(matched) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matched))
	}
	if matched[0].ID != 1 {
		t.Errorf("Expected quota ID 1, got %d", matched[0].ID)
	}
}

func TestMatchQuotas_WildcardAlias(t *testing.T) {
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
	})

	matched := matchQuotas(snap, 10, "any-model")
	if len(matched) != 1 {
		t.Fatalf("Wildcard should match any alias, got %d matches", len(matched))
	}
}

func TestMatchQuotas_WildcardAPIKey(t *testing.T) {
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 0, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
	})

	// APIKeyID=0 means all keys
	matched := matchQuotas(snap, 999, "gpt-4o")
	if len(matched) != 1 {
		t.Errorf("Wildcard API key should match any key, got %d", len(matched))
	}
}

func TestMatchQuotas_NoMatch(t *testing.T) {
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
	})

	matched := matchQuotas(snap, 999, "gpt-4o")
	if len(matched) != 0 {
		t.Errorf("Different API key should not match, got %d", len(matched))
	}
}

func TestMatchQuotas_MultipleMatches(t *testing.T) {
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
		{ID: 2, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "tokens", Period: "month", LimitValue: 10000, Enabled: true},
		{ID: 3, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day", LimitValue: 200, Enabled: true},
	})

	matched := matchQuotas(snap, 10, "gpt-4o")
	if len(matched) != 3 {
		t.Errorf("Expected 3 matches, got %d", len(matched))
	}
}

// ========================
// RateLimiter.Allow 测试
// ========================

func TestRateLimiter_Allow_UnderLimit(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 5, Enabled: true},
	}, nil)

	for i := 0; i < 5; i++ {
		ok, rule := rl.Allow(snap, 1, "gpt-4o")
		if !ok {
			t.Errorf("Request %d should be allowed, got blocked by rule %v", i+1, rule)
		}
	}
}

func TestRateLimiter_Allow_OverLimit(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 3, Enabled: true},
	}, nil)

	// First 3 requests pass
	for i := 0; i < 3; i++ {
		ok, _ := rl.Allow(snap, 1, "gpt-4o")
		if !ok {
			t.Fatalf("Request %d should be allowed", i+1)
		}
	}

	// 4th request blocked
	ok, rule := rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Error("4th request should be blocked")
	}
	if rule == nil {
		t.Error("Should return the blocking rule")
	}
	if rule.ID != 1 {
		t.Errorf("Expected rule ID 1, got %d", rule.ID)
	}
}

func TestRateLimiter_Allow_DifferentAPIKeys(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 2, Enabled: true},
	}, nil)

	// API key 1 uses 2 requests
	rl.Allow(snap, 1, "gpt-4o")
	rl.Allow(snap, 1, "gpt-4o")

	// API key 1 is now blocked
	ok, _ := rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Error("API key 1 should be blocked")
	}

	// API key 2 should still be allowed (separate counter)
	ok, _ = rl.Allow(snap, 2, "gpt-4o")
	if !ok {
		t.Error("API key 2 should be allowed (separate counter)")
	}
}

func TestRateLimiter_Allow_SpecificAPIKey(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 10, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 2, Enabled: true},
	}, nil)

	// API key 10 is limited
	ok, _ := rl.Allow(snap, 10, "gpt-4o")
	if !ok {
		t.Error("First request for key 10 should pass")
	}

	// API key 99 is not affected by this rule
	ok, _ = rl.Allow(snap, 99, "gpt-4o")
	if !ok {
		t.Error("API key 99 should not be affected by key-10-specific rule")
	}
}

func TestRateLimiter_Allow_SpecificModelAlias(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "gpt-4o", WindowSeconds: 60, MaxRequests: 1, Enabled: true},
	}, nil)

	// gpt-4o is limited to 1
	ok, _ := rl.Allow(snap, 1, "gpt-4o")
	if !ok {
		t.Error("First gpt-4o request should pass")
	}
	ok, _ = rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Error("Second gpt-4o request should be blocked")
	}

	// claude-3 is not affected
	ok, _ = rl.Allow(snap, 1, "claude-3")
	if !ok {
		t.Error("claude-3 should not be affected by gpt-4o rule")
	}
}

func TestRateLimiter_Allow_WindowReset(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 1, MaxRequests: 1, Enabled: true},
	}, nil)

	// Use up the limit
	ok, _ := rl.Allow(snap, 1, "gpt-4o")
	if !ok {
		t.Fatal("First request should pass")
	}
	ok, _ = rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Fatal("Second request should be blocked")
	}

	// Wait for window to reset
	time.Sleep(1100 * time.Millisecond)

	ok, _ = rl.Allow(snap, 1, "gpt-4o")
	if !ok {
		t.Error("Request after window reset should pass")
	}
}

func TestRateLimiter_Allow_MultipleRules(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 10, Enabled: true},
		{ID: 2, APIKeyID: 0, ModelAlias: "gpt-4o", WindowSeconds: 60, MaxRequests: 2, Enabled: true},
	}, nil)

	// The stricter rule (2) should trigger first
	rl.Allow(snap, 1, "gpt-4o")
	rl.Allow(snap, 1, "gpt-4o")

	ok, rule := rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Error("3rd gpt-4o request should be blocked by stricter rule")
	}
	if rule != nil && rule.ID != 2 {
		t.Errorf("Expected rule 2 (stricter), got rule %d", rule.ID)
	}
}

func TestRateLimiter_Allow_NoRules(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap(nil, nil)

	ok, rule := rl.Allow(snap, 1, "gpt-4o")
	if !ok {
		t.Error("No rules = should allow")
	}
	if rule != nil {
		t.Error("No rules = should return nil rule")
	}
}

func TestRateLimiter_Allow_ZeroMaxRequests(t *testing.T) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 0, Enabled: true},
	}, nil)

	// max1(0) = 1, so first request passes, second blocked
	ok, _ := rl.Allow(snap, 1, "gpt-4o")
	if !ok {
		t.Error("First request should pass (max1(0)=1)")
	}
	ok, _ = rl.Allow(snap, 1, "gpt-4o")
	if ok {
		t.Error("Second request should be blocked")
	}
}

// ========================
// QuotaManager.Check 测试 (需要 DB)
// ========================

func TestQuotaManager_Check_UnderLimit(t *testing.T) {
	gdb := mustOpenDB(t)

	now := time.Now()
	reset := now.Add(24 * time.Hour)
	gdb.Create(&model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 50, ResetAt: reset, Enabled: true,
	})

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
	})

	ok, over := qm.Check(snap, 10, "gpt-4o")
	if !ok {
		t.Errorf("Should pass (50/100), got over: %+v", over)
	}
}

func TestQuotaManager_Check_OverLimit(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	now := time.Now()
	reset := now.Add(24 * time.Hour)
	gdb.Create(&model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 100, ResetAt: reset, Enabled: true,
	})

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 100, Enabled: true},
	})

	ok, over := qm.Check(snap, 10, "gpt-4o")
	if ok {
		t.Error("Should be blocked (100/100)")
	}
	if over == nil {
		t.Fatal("Should return OverInfo")
	}
	if over.QuotaID != 1 {
		t.Errorf("Expected QuotaID 1, got %d", over.QuotaID)
	}
	if over.Used != 100 || over.Limit != 100 {
		t.Errorf("Used=%d, Limit=%d, want 100/100", over.Used, over.Limit)
	}
}

func TestQuotaManager_Check_PeriodExpired(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	// ResetAt in the past → should be treated as reset (used=0)
	gdb.Create(&model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day",
		LimitValue: 10, UsedValue: 10, ResetAt: time.Now().Add(-1 * time.Hour), Enabled: true,
	})

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day", LimitValue: 10, Enabled: true},
	})

	ok, _ := qm.Check(snap, 10, "gpt-4o")
	if !ok {
		t.Error("Should pass because period expired (treated as reset)")
	}
}

func TestQuotaManager_Check_NoQuota(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, nil)

	ok, over := qm.Check(snap, 10, "gpt-4o")
	if !ok {
		t.Error("No quota = should pass")
	}
	if over != nil {
		t.Error("No quota = should return nil OverInfo")
	}
}

func TestQuotaManager_Check_DegradeAction(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	reset := time.Now().Add(24 * time.Hour)
	gdb.Create(&model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 10, UsedValue: 10, ResetAt: reset, OverAction: "degrade",
		DegradeAlias: "gpt-4o-mini", Enabled: true,
	})

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day", LimitValue: 10, OverAction: "degrade", DegradeAlias: "gpt-4o-mini", Enabled: true},
	})

	ok, over := qm.Check(snap, 10, "gpt-4o")
	if ok {
		t.Error("Should be over limit")
	}
	if over == nil {
		t.Fatal("Should return OverInfo")
	}
	if over.OverAction != "degrade" {
		t.Errorf("Expected degrade action, got %s", over.OverAction)
	}
	if over.DegradeAlias != "gpt-4o-mini" {
		t.Errorf("Expected degrade alias gpt-4o-mini, got %s", over.DegradeAlias)
	}
}

// ========================
// QuotaManager.Consume 测试
// ========================

func TestQuotaManager_Consume_Requests(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	reset := time.Now().Add(24 * time.Hour)
	q := model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 0, ResetAt: reset, Enabled: true,
	}
	gdb.Create(&q)

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{q})

	qm.Consume(snap, 10, "gpt-4o", 0, 0)

	var updated model.Quota
	gdb.First(&updated, 1)
	if updated.UsedValue != 1 {
		t.Errorf("After consume, UsedValue = %d, want 1", updated.UsedValue)
	}
}

func TestQuotaManager_Consume_Tokens(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	reset := time.Now().Add(24 * time.Hour)
	q := model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "tokens", Period: "month",
		LimitValue: 10000, UsedValue: 0, ResetAt: reset, Enabled: true,
	}
	gdb.Create(&q)

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{q})

	qm.Consume(snap, 10, "gpt-4o", 500, 200)

	var updated model.Quota
	gdb.First(&updated, 1)
	if updated.UsedValue != 700 {
		t.Errorf("After consume, UsedValue = %d, want 700 (500+200)", updated.UsedValue)
	}
}

func TestQuotaManager_Consume_LazyReset(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	// ResetAt in the past → should reset then add
	q := model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 99, ResetAt: time.Now().Add(-1 * time.Hour), Enabled: true,
	}
	gdb.Create(&q)

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{q})

	qm.Consume(snap, 10, "gpt-4o", 0, 0)

	var updated model.Quota
	gdb.First(&updated, 1)
	if updated.UsedValue != 1 {
		t.Errorf("After lazy reset + consume, UsedValue = %d, want 1", updated.UsedValue)
	}
	// ResetAt should be updated to future
	if !updated.ResetAt.After(time.Now()) {
		t.Error("ResetAt should be in the future after lazy reset")
	}
}

func TestQuotaManager_Consume_Multiple(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	reset := time.Now().Add(24 * time.Hour)
	gdb.Create(&model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 5, ResetAt: reset, Enabled: true,
	})
	gdb.Create(&model.Quota{
		ID: 2, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "tokens", Period: "month",
		LimitValue: 10000, UsedValue: 100, ResetAt: reset, Enabled: true,
	})

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{
		{ID: 1, APIKeyID: 10, ModelAlias: "*", QuotaType: "requests", Period: "day", LimitValue: 100, ResetAt: reset, Enabled: true},
		{ID: 2, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "tokens", Period: "month", LimitValue: 10000, ResetAt: reset, Enabled: true},
	})

	qm.Consume(snap, 10, "gpt-4o", 100, 50)

	var q1, q2 model.Quota
	gdb.First(&q1, 1)
	gdb.First(&q2, 2)

	if q1.UsedValue != 6 {
		t.Errorf("Quota 1 (requests): UsedValue = %d, want 6", q1.UsedValue)
	}
	if q2.UsedValue != 250 {
		t.Errorf("Quota 2 (tokens): UsedValue = %d, want 250 (100+150)", q2.UsedValue)
	}
}

func TestQuotaManager_Consume_NoMatch(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	q := model.Quota{
		ID: 1, APIKeyID: 10, ModelAlias: "gpt-4o", QuotaType: "requests", Period: "day",
		LimitValue: 100, UsedValue: 0, ResetAt: time.Now().Add(24 * time.Hour), Enabled: true,
	}
	gdb.Create(&q)

	qm := NewQuotaManager(gdb)
	snap := makeSnap(nil, []model.Quota{q})

	// Different API key → no match → no consume
	qm.Consume(snap, 999, "gpt-4o", 100, 100)

	var updated model.Quota
	gdb.First(&updated, 1)
	if updated.UsedValue != 0 {
		t.Errorf("No match: UsedValue should remain 0, got %d", updated.UsedValue)
	}
}

// ========================
// RateLimiter.FlushHits 测试
// ========================

func TestRateLimiter_FlushHits(t *testing.T) {
	gdb := mustOpenDB(t)
	defer closeDB(gdb)

	gdb.Create(&model.RateLimitRule{
		ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 1, Enabled: true,
	})

	rl := NewRateLimiter(gdb)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 1, Enabled: true},
	}, nil)

	// Trigger 3 rejections
	for i := 0; i < 4; i++ {
		rl.Allow(snap, 1, "gpt-4o") // 1 passes, 3 blocked
	}

	rl.FlushHits()

	var rule model.RateLimitRule
	gdb.First(&rule, 1)
	if rule.TotalHits < 3 {
		t.Errorf("Expected TotalHits >= 3, got %d", rule.TotalHits)
	}
}

// ========================
// 基准测试
// ========================

func BenchmarkNextReset_Day(b *testing.B) {
	now := time.Date(2026, 9, 2, 15, 30, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NextReset("day", now)
	}
}

func BenchmarkNextReset_Week(b *testing.B) {
	now := time.Date(2026, 9, 2, 15, 30, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NextReset("week", now)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 1000000, Enabled: true},
	}, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(snap, 1, "gpt-4o")
	}
}

func BenchmarkRateLimiter_Allow_MultipleRules(b *testing.B) {
	rl := NewRateLimiter(nil)
	snap := makeSnap([]model.RateLimitRule{
		{ID: 1, APIKeyID: 0, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 1000000, Enabled: true},
		{ID: 2, APIKeyID: 0, ModelAlias: "gpt-4o", WindowSeconds: 60, MaxRequests: 1000000, Enabled: true},
		{ID: 3, APIKeyID: 1, ModelAlias: "*", WindowSeconds: 60, MaxRequests: 1000000, Enabled: true},
	}, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(snap, 1, "gpt-4o")
	}
}
