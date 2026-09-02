// Package metrics 运行时轻量指标采集（REQ-016/018：QPS、连接数、错误环形缓冲）。
package metrics

import (
	"sort"
	"sync"
	"time"
)

type ModelStat struct {
	buckets [60]int64 // 最近 60 秒每秒请求数
	errs    [60]int64
	lastSec int64
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
	mu       sync.Mutex
	stats    map[string]*ModelStat
	recent   []ErrorEvent
	conns    int64
	start    time.Time
	todayCalls   int64
	todayBlocked int64
}

func New() *Metrics {
	return &Metrics{stats: map[string]*ModelStat{}, start: time.Now()}
}

func (mt *Metrics) Observe(model string, isErr bool) {
	now := time.Now().Unix()
	mt.mu.Lock()
	defer mt.mu.Unlock()
	s := mt.stats[model]
	if s == nil {
		s = &ModelStat{lastSec: now}
		mt.stats[model] = s
	}
	s.rotate(now)
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
	Model   string  `json:"model"`
	QPS     float64 `json:"qps"`
	Errs1m  int64   `json:"errors_1m"`
}

func (mt *Metrics) QPS() []QPSInfo {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	out := make([]QPSInfo, 0, len(mt.stats))
	for m, s := range mt.stats {
		s.rotate(time.Now().Unix())
		var total, errs int64
		for i := 0; i < 60; i++ {
			total += s.buckets[i]
			errs += s.errs[i]
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
