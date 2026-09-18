// prom.go Prometheus 文本出口。
// 设计约束：
//   - 零第三方依赖：手写 Prometheus 文本格式 v0.0.4（/metrics 抓取的事实标准，
//     Prometheus server / VictoriaMetrics / vmagent 均直接支持）；
//   - 热路径零影响：指标收集走 metrics 包既有原子计数；SLB/健康数据通过
//     PromCollector 接口在抓取瞬间以回调拉取（admin 包注册，复用 HealthList 语义）；
//   - 抓取加互斥锁：HealthList 内部取 bl.mu，避免多抓取并发竞争放大；
//     Go runtime 指标由 runtime/metrics 一次快照导出。

package metrics

import (
	"fmt"
	"io"
	"runtime/metrics"
	"strings"
)

// PromSnapshot 抓取瞬间的指标快照（由 Server 端组装）。
type PromSnapshot struct {
	// ModelQPS：模型维度 QPS/错误快照；nil 时回退 mt.QPS()。
	ModelQPS []QPSInfo
	// Upstreams：每个供应商一行的健康/熔断状态（admin 注册时经 slb.HealthList 提供）。
	Upstreams []PromUpstream
}

// PromUpstream 单供应商健康行。
type PromUpstream struct {
	Name     string
	Protocol string
	Healthy  bool
	Fails    int
}

// PromCollector 由 admin.Server 实现：抓取时组装快照（含 SLB 健康列表）。
type PromCollector interface {
	PromSnapshot() PromSnapshot
}

// promMu 序列化 /metrics 抓取（HealthList 抓 bl.mu，防多抓取并发竞争）——见 Metrics 字段。

// promEsc 转义 label 值与 help 文本（Prometheus 文本格式规范）。
func promEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// writeMetric 输出一条 counter/gauge（可选 label）。
func writeMetric(w io.Writer, name, typ, help, labels string, value float64) {
	if help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", name, promEsc(help))
	}
	fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
	if labels != "" {
		fmt.Fprintf(w, "%s{%s} %s\n", name, labels, formatValue(value))
	} else {
		fmt.Fprintf(w, "%s %s\n", name, formatValue(value))
	}
}

func formatValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

// goRuntimeSamples：runtime/metrics 样本 → (导出名, 类型, help)。
var goRuntimeSamples = []struct {
	name   string // runtime/metrics 样本名
	export string // Prometheus 指标名
	typ    string // counter | gauge
	help   string
}{
	{"/gc/heap/allocs:bytes", "go_gc_heap_allocs_bytes_total", "counter", "Cumulative heap bytes allocated"},
	{"/gc/heap/frees:bytes", "go_gc_heap_frees_bytes_total", "counter", "Cumulative heap bytes freed"},
	{"/gc/cycles/total:gc-cycles", "go_gc_cycles_total", "counter", "Completed GC cycles"},
	{"/memory/classes/heap/free:bytes", "go_memstats_heap_free_bytes", "gauge", "Heap free bytes"},
	{"/memory/classes/heap/objects:bytes", "go_memstats_heap_inuse_bytes", "gauge", "Heap objects bytes"},
	{"/memory/classes/heap/released:bytes", "go_memstats_heap_released_bytes", "gauge", "Heap released bytes"},
	{"/memory/classes/heap/stacks:bytes", "go_memstats_heap_stack_bytes", "gauge", "Heap stack bytes"},
	{"/sync/mutex/wait/total:seconds", "go_mutex_wait_seconds_total", "counter", "Cumulative mutex wait seconds"},
	{"/sched/goroutines-created:goroutines", "go_goroutines_created_total", "counter", "Goroutines created"},
}

// goRuntimeMetrics 输出 go_* 指标（一次 runtime/metrics 快照；缺失样本跳过）。
func goRuntimeMetrics(w io.Writer) {
	samples := make([]metrics.Sample, len(goRuntimeSamples))
	for i, s := range goRuntimeSamples {
		samples[i] = metrics.Sample{Name: s.name}
	}
	metrics.Read(samples)
	for i, s := range goRuntimeSamples {
		v := samples[i].Value
		switch v.Kind() {
		case metrics.KindUint64:
			writeMetric(w, s.export, s.typ, s.help, "", float64(v.Uint64()))
		case metrics.KindFloat64:
			writeMetric(w, s.export, s.typ, s.help, "", v.Float64())
		default: // KindBad / 其它：该平台不提供此样本，跳过
			continue
		}
	}
	// goroutines 当前值（瞬时 gauge）
	ng := []metrics.Sample{{Name: "/sched/goroutines:goroutines"}}
	metrics.Read(ng)
	if ng[0].Value.Kind() == metrics.KindUint64 {
		writeMetric(w, "go_goroutines", "gauge", "Number of goroutines", "", float64(ng[0].Value.Uint64()))
	}
}

// WritePromRender 输出完整 /metrics 文本（业务 + go runtime）。
func (mt *Metrics) WritePromRender(w io.Writer, col PromCollector) {
	mt.promMu.Lock()
	defer mt.promMu.Unlock()

	var snap PromSnapshot
	if col != nil {
		snap = col.PromSnapshot()
	}
	qps := snap.ModelQPS
	if qps == nil {
		qps = mt.QPS()
	}

	// ---- 进程/运行状态 ----
	writeMetric(w, "gw_uptime_seconds", "gauge", "Gateway process uptime in seconds", "", mt.Uptime().Seconds())
	writeMetric(w, "gw_active_connections", "gauge", "Currently active gateway connections", "", float64(mt.Conns()))

	// ---- 模型维度 QPS / 错误（60s 窗口快照）----
	totalQPS := 0.0
	for _, q := range qps {
		totalQPS += q.QPS
	}
	writeMetric(w, "gw_requests_per_second", "gauge", "Requests per second across all models (60s window)", "", totalQPS)
	for _, q := range qps {
		l := `model="` + promEsc(q.Model) + `"`
		writeMetric(w, "gw_model_requests_per_second", "gauge", "Per-model requests per second (60s window)", l, q.QPS)
		writeMetric(w, "gw_model_errors_1m", "gauge", "Per-model errors in the last minute", l, float64(q.Errs1m))
	}

	// ---- 上游供应商健康/熔断（admin 注册 PromCollector 提供；未注册则无该段）----
	for _, u := range snap.Upstreams {
		l := `provider="` + promEsc(u.Name) + `",protocol="` + promEsc(u.Protocol) + `"`
		hv := 0.0
		if u.Healthy {
			hv = 1
		}
		writeMetric(w, "gw_upstream_healthy", "gauge", "Upstream provider reachable (1=healthy, 0=circuit open/dead)", l, hv)
		writeMetric(w, "gw_upstream_consecutive_failures", "gauge", "Consecutive upstream failures (circuit breaker counter)", l, float64(u.Fails))
	}

	// ---- Go runtime ----
	goRuntimeMetrics(w)
}
