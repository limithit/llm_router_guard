// Package metrics 运行时轻量指标采集（REQ-016/018：QPS、连接数、错误环形缓冲）。
package metrics

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type ModelStat struct {
	buckets [60]int64 // 最近 60 秒每秒请求数
	errs    [60]int64
	lastSec int64
	lastObs int64 // 最近一次被 Observe 的时刻（QPS 轮询会推进 lastSec，不能充当空闲判据）
}

func (m *ModelStat) rotate(now int64) {
	if now <= m.lastSec {
		return
	}
	shift := now - m.lastSec
	if shift >= 60 {
		m.buckets = [60]int64{}
		m.errs = [60]int64{}
	} else {
		for i := int64(0); i < shift; i++ {
			copy(m.buckets[:], m.buckets[1:])
			m.buckets[59] = 0
			copy(m.errs[:], m.errs[1:])
			m.errs[59] = 0
		}
	}
	m.lastSec = now
}

type ErrorEvent struct {
	Time      time.Time `json:"time"`
	RequestID string    `json:"request_id"`
	Message   string    `json:"message"`
}

type Metrics struct {
	mu           sync.Mutex
	stats        map[string]*ModelStat
	recent       []ErrorEvent
	conns        int64
	start        time.Time
	todayCalls   int64
	todayBlocked int64
	promMu       sync.Mutex // 序列化 /metrics 抓取（HealthList 内部取 bl.mu，防抓取并发竞争）
}

func New() *Metrics {
	return &Metrics{stats: map[string]*ModelStat{}, start: time.Now()}
}

// SEC-05：stats map 必须有界 —— model 标签此前直接取未经校验的请求体 cr.Model（任意 UTF-8、
// 404 路径也记账且不吃限流），匿名 Key 可无限撑大 map 并拖慢 /metrics 渲染（内存+CPU DoS）。
// 标签清洗：去控制字符、限长；基数超过上限后归并到 overflow 桶。
const (
	maxModelLabels    = 512
	overflowLabel     = "*"
	maxModelLabelByte = 64
)

func sanitizeModelLabel(m string) string {
	if len(m) > maxModelLabelByte {
		m = m[:maxModelLabelByte]
	}
	var b strings.Builder
	b.Grow(len(m))
	for _, r := range m {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (mt *Metrics) Observe(model string, isErr bool) {
	model = sanitizeModelLabel(model)
	now := time.Now().Unix()
	mt.mu.Lock()
	defer mt.mu.Unlock()
	s := mt.stats[model]
	if s == nil {
		if len(mt.stats) >= maxModelLabels {
			model = overflowLabel // 新标签不再单独建目：合并计入通配桶
			s = mt.stats[model]
		}
		if s == nil {
			s = &ModelStat{lastSec: now}
			mt.stats[model] = s
		}
	}
	s.rotate(now)
	s.lastObs = now
	idx := now % 60
	s.buckets[idx]++
	if isErr {
		s.errs[idx]++
	}
}

func (mt *Metrics) RecordError(requestID, msg string) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.recent = append(mt.recent, ErrorEvent{Time: time.Now(), RequestID: requestID, Message: msg})
	if len(mt.recent) > 50 {
		mt.recent = mt.recent[len(mt.recent)-50:]
	}
}

type QPSInfo struct {
	Model  string  `json:"model"`
	QPS    float64 `json:"qps"`
	Errs1m int64   `json:"errors_1m"`
}

func (mt *Metrics) QPS() []QPSInfo {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	out := make([]QPSInfo, 0, len(mt.stats))
	now := time.Now().Unix()
	for m, s := range mt.stats {
		s.rotate(now)
		var total, errs int64
		for i := 0; i < 60; i++ {
			total += s.buckets[i]
			errs += s.errs[i]
		}
		// SEC-05 配套：超过 10 分钟无观测且窗口内无流量的标签回收（基数可真正有界）
		if total == 0 && now-s.lastSec > 600 && m != overflowLabel {
			delete(mt.stats, m)
			continue
		}
		out = append(out, QPSInfo{Model: m, QPS: float64(total) / 60, Errs1m: errs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QPS > out[j].QPS })
	return out
}

func (mt *Metrics) RecentErrors() []ErrorEvent {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	cp := make([]ErrorEvent, len(mt.recent))
	copy(cp, mt.recent)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Time.After(cp[j].Time) })
	return cp
}

func (mt *Metrics) ConnInc() { mt.mu.Lock(); mt.conns++; mt.mu.Unlock() }
func (mt *Metrics) ConnDec() {
	mt.mu.Lock()
	if mt.conns > 0 {
		mt.conns--
	}
	mt.mu.Unlock()
}
func (mt *Metrics) Conns() int64 { mt.mu.Lock(); defer mt.mu.Unlock(); return mt.conns }

func (mt *Metrics) Uptime() time.Duration { return time.Since(mt.start) }
