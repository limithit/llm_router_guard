package tokens

import (
	"testing"
)

// TestCount_EmptyString 空串应返回 0。
func TestCount_EmptyString(t *testing.T) {
	if got := Count(""); got != 0 {
		t.Errorf("Count(\"\") = %d, want 0", got)
	}
}

// TestCount_English cl100k_base 下 "hello world" = 2 tokens。
func TestCount_English(t *testing.T) {
	got := Count("hello world")
	if got < 1 {
		t.Fatalf("Count(\"hello world\") = %d, want >= 1", got)
	}
	// 精确值依赖编码器是否可用：
	if Available() && got != 2 {
		t.Errorf("Count(\"hello world\") = %d, want 2 (cl100k_base)", got)
	}
}

// TestCount_Chinese 中文输出应为正数。
func TestCount_Chinese(t *testing.T) {
	got := Count("你好世界，这是一段中文测试文本")
	if got <= 0 {
		t.Errorf("Count(Chinese) = %d, want > 0", got)
	}
}

// TestEstimate 启发式回退对空串、CJK 混合、纯 ASCII 的基本正确性。
func TestEstimate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want func(int) bool
	}{
		{"empty", "", func(n int) bool { return n == 1 /* 至少记 1 */ }},
		{"ascii-only", "hello world, this is a longer english sentence", func(n int) bool { return n > 0 }},
		{"chinese", "你好世界这是一段中文", func(n int) bool { return n > 0 }},
		{"mixed", "hello 你好 world 世界", func(n int) bool { return n > 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Estimate(tc.in)
			if !tc.want(got) {
				t.Errorf("Estimate(%q) = %d, failed want predicate", tc.in, got)
			}
		})
	}
}

// TestCount_CJKHeuristic 中文字符按 cjk/2 + other/4 + 1 估算，验证边界。
func TestEstimate_CJKRatio(t *testing.T) {
	// 10 个 CJK 字符 → 至少 5（10/2+1）
	in := "你好世界测试测试测试" // 10 runes, all CJK
	got := Estimate(in)
	if got < 6 {
		t.Errorf("Estimate(10 CJK) = %d, want >= 6 (cjk/2+1)", got)
	}
}
