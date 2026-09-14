// Package health 提供上游健康检查定时任务（P1 #3）：
// 周期探测所有启用供应商（GET /v1/models），结果通过 slb 熔断计数器记录——
// 传输层失败（超时/DNS/连接拒绝）累计到阈值即打开熔断（多实例部署时经 Redis 广播），
// 恢复后自动清除。判据刻意宽松：任何 HTTP 响应（含 401/403/404）= 端点可达 = 健康，
// 认证错误属于"测试连接"（严格判据）的职责。探测不进审计、不占配额；
// 健康状态经 /status 与 dashboard 的 upstream_health 展示。
package health

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/crypto"
	"llmrouter/internal/httpclient"
	"llmrouter/internal/model"
	"llmrouter/internal/settings"
	"llmrouter/internal/slb"
)

const defaultConcurrent = 8 // 单轮并发探测上限，避免大量供应商时瞬时风暴

type Checker struct {
	db     *gorm.DB
	enc    *crypto.Cipher
	bl     *slb.Balancer
	client *http.Client
}

// New 构造健康检查器。db 用于读取供应商列表与故障转移配置。
// M-01：经 httpclient 工厂（拒绝重定向，供应商 Key 不随 302 外泄）。
func New(db *gorm.DB, enc *crypto.Cipher, bl *slb.Balancer) *Checker {
	return &Checker{db: db, enc: enc, bl: bl,
		client: httpclient.New(10 * time.Second)}
}

// Run 健康检查循环；interval<=0 视为禁用（直接返回）。
// 启动时先立即探测一轮，让熔断状态尽早反映上游真实可达性。
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	c.ProbeAll(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.ProbeAll(ctx)
		}
	}
}

// ProbeAll 对所有启用供应商做一轮探测（有界并发）。
func (c *Checker) ProbeAll(ctx context.Context) {
	var ps []model.Provider
	if err := c.db.Where("enabled = ?", true).Find(&ps).Error; err != nil {
		return
	}
	if len(ps) == 0 {
		return
	}
	fo := settings.Failover{}
	settings.LoadKV(c.db, model.SetKeyFailover, &fo, settings.DefaultFailover())

	sem := make(chan struct{}, defaultConcurrent)
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		sem <- struct{}{}
		go func(p model.Provider) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[health] probe %s panic recovered: %v", p.Name, r)
				}
			}()
			c.probe(ctx, p, fo)
		}(p)
	}
	wg.Wait()
}

// probe 探测单个供应商。传输层错误 → RecordFailure（累计开熔断）；
// 有 HTTP 响应 → RecordSuccess（清计数/清共享打开态）。
func (c *Checker) probe(ctx context.Context, p model.Provider, fo settings.Failover) {
	base := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		c.bl.RecordFailure(p.ID, fo, "healthcheck build request: "+err.Error())
		return
	}
	if k := c.apiKey(p); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.bl.RecordFailure(p.ID, fo, "healthcheck: "+err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	c.bl.RecordSuccess(p.ID)
}

// apiKey 解密供应商密钥；失败返回空串（探测仍可进行，可达性判据不依赖认证）。
func (c *Checker) apiKey(p model.Provider) string {
	if p.APIKeyEnc == "" {
		return ""
	}
	k, err := c.enc.Decrypt(p.APIKeyEnc)
	if err != nil {
		return ""
	}
	return k
}
