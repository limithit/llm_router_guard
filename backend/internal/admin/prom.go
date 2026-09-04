// prom.go Prometheus 抓取的 admin 侧适配（P2 #6）：
// Server 实现 metrics.PromCollector，在抓取瞬间组装 SLB 健康行
// （复用 slb.HealthList 语义：本地熔断 + Redis 共享打开状态）。

package admin

import (
	"llmrouter/internal/metrics"
)

// PromSnapshot 组装 /metrics 快照（metrics.PromCollector 接口实现）。
func (s *Server) PromSnapshot() metrics.PromSnapshot {
	snap := metrics.PromSnapshot{Upstreams: []metrics.PromUpstream{}}
	if s.mx != nil {
		snap.ModelQPS = s.mx.QPS()
	}
	if s.mgr != nil && s.bl != nil {
		if st := s.mgr.Get(); st != nil {
			for _, h := range s.bl.HealthList(st) {
				snap.Upstreams = append(snap.Upstreams, metrics.PromUpstream{
					Name:     h.Provider,
					Protocol: h.Protocol,
					Healthy:  h.Healthy,
					Fails:    h.FailCount,
				})
			}
		}
	}
	return snap
}
