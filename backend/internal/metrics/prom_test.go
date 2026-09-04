package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromEsc(t *testing.T) {
	cases := map[string]string{
		`plain`:      `plain`,
		`a"b`:        `a\"b`,
		`a\b`:        `a\\b`,
		"line\nnext": `line\nnext`,
	}
	for in, want := range cases {
		if got := promEsc(in); got != want {
			t.Errorf("promEsc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatValue(t *testing.T) {
	if got := formatValue(42); got != "42" {
		t.Errorf("formatValue(42) = %q, want 42", got)
	}
	if got := formatValue(0.25); got != "0.25" {
		t.Errorf("formatValue(0.25) = %q, want 0.25", got)
	}
	if got := formatValue(1.5e6); got == "" || !strings.ContainsAny(got, "e") && got != "1500000" {
		t.Logf("formatValue(1.5e6) = %q", got) // %g 或整型形式均可，仅观测
	}
}

func TestWriteMetric(t *testing.T) {
	var b bytes.Buffer
	writeMetric(&b, "gw_upstream_healthy", "gauge", `reaches (1=ok)`, `provider="a\"b"`, 1)
	out := b.String()
	for _, want := range []string{
		"# HELP gw_upstream_healthy reaches (1=ok)\n",
		"# TYPE gw_upstream_healthy gauge\n",
		"gw_upstream_healthy{provider=\"a\\\"b\"} 1\n",
	} {
		if !strings.Contains(out, strings.ReplaceAll(want, `\n`, "\n")) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestWritePromRender_NoCollector(t *testing.T) {
	mt := New()
	mt.Observe("gpt-4o", false)
	mt.Observe("gpt-4o", true)
	mt.ConnInc()

	var b bytes.Buffer
	mt.WritePromRender(&b, nil)
	out := b.String()

	for _, want := range []string{
		"# HELP gw_uptime_seconds",
		"# TYPE gw_active_connections gauge",
		"gw_active_connections 1\n",
		"gw_requests_per_second 0.0",
		`gw_model_requests_per_second{model="gpt-4o"} 0.0`,
		`gw_model_errors_1m{model="gpt-4o"} 1`,
		"# HELP go_goroutines", // runtime 段存在
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\ngot:\n%s", want, out)
		}
	}
	if strings.Contains(out, "gw_upstream_healthy") {
		t.Errorf("no collector should omit upstream section")
	}
}

func TestWritePromRender_WithCollector(t *testing.T) {
	mt := New()
	col := fakeCollector{}
	var b bytes.Buffer
	mt.WritePromRender(&b, col)
	out := b.String()

	for _, want := range []string{
		`gw_upstream_healthy{provider="openai-main",protocol="openai_chat"} 1`,
		`gw_upstream_healthy{provider="bad \"quoted\"",protocol="anthropic"} 0`,
		`gw_upstream_consecutive_failures{provider="openai-main",protocol="openai_chat"} 0`,
		`gw_upstream_consecutive_failures{provider="bad \"quoted\"",protocol="anthropic"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\ngot:\n%s", want, out)
		}
	}
}

type fakeCollector struct{}

func (fakeCollector) PromSnapshot() PromSnapshot {
	return PromSnapshot{
		ModelQPS: []QPSInfo{{Model: "gpt-4o", QPS: 2.5, Errs1m: 1}},
		Upstreams: []PromUpstream{
			{Name: "openai-main", Protocol: "openai_chat", Healthy: true, Fails: 0},
			{Name: "bad \"quoted\"", Protocol: "anthropic", Healthy: false, Fails: 3},
		},
	}
}
