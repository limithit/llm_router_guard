// Package runtime 实现 PRD 6.3.2 配置热加载机制：
//   - 所有配置以数据库为准，加载进内存形成不可变快照；
//   - 管理 API 写入后 Bump(counter) 并 Trigger 即时重载（实时触发）；
//   - 后台 goroutine 按 hot_reload_seconds 轮询 DB config_meta.counter（多实例/外部改库兜底）；
//   - 原子替换快照：进行中请求继续持有旧快照指针，新请求使用新配置（≤3 秒生效，REQ-004）；
//   - 每次重载生成 ConfigVersion（含全量快照 JSON，供一键回滚 REQ-004A）与逐模块加载日志。
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
	"llmrouter/internal/settings"
)

// ---- 编译后的规则 ----

type CompiledKeyword struct {
	Raw model.GuardKeyword
	Re  *regexp.Regexp // regex 模式时非空
}

type CompiledPII struct {
	Raw model.PIIRule
	Re  *regexp.Regexp
}

type CompiledInjection struct {
	Raw model.InjectionRule
	Re  *regexp.Regexp
}

// ResolvedUpstream 别名解析后的一个可选上游。
type ResolvedUpstream struct {
	ProviderID    uint   `json:"provider_id"`
	ProviderName  string `json:"provider_name"`
	Protocol      string `json:"protocol"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"-"`
	UpstreamModel string `json:"upstream_model"`
	Weight        int    `json:"weight"`
}

// Snapshot 是网关请求路径读取的唯一配置视图（加载后不可变）。
type Snapshot struct {
	Version        string
	LoadedAt       time.Time
	Providers      map[uint]*model.Provider
	ProviderByName map[string]*model.Provider
	Aliases        map[string][]ResolvedUpstream
	APIKeys        map[string]*model.APIKey // key hash hex -> record
	Keywords       []CompiledKeyword
	PIIRules       []CompiledPII
	Injection      []CompiledInjection
	Quotas         []model.Quota
	RateLimits     []model.RateLimitRule
	General        settings.General
	Failover       settings.Failover
	Output         settings.OutputFilter
	Security       settings.Security
	Alerts         settings.QuotaAlerts
	Counts         map[string]int
	LoadErrors     []string
}

type Manager struct {
	db  *gorm.DB
	enc *crypto.Cipher
	cur atomic.Pointer[Snapshot]

	trigger chan struct{}
	busy    atomic.Bool

	mu      sync.Mutex
	dbCount int64 // 上次加载时 DB 中的 counter
	Version string
	Status  string
	Err     string
}

func NewManager(gdb *gorm.DB, enc *crypto.Cipher) *Manager {
	m := &Manager{db: gdb, enc: enc, trigger: make(chan struct{}, 1)}
	m.cur.Store(emptySnapshot())
	return m
}

func emptySnapshot() *Snapshot {
	return &Snapshot{
		Version: "-", LoadedAt: time.Now(),
		Counts:    map[string]int{},
		Providers: map[uint]*model.Provider{}, ProviderByName: map[string]*model.Provider{},
		Aliases: map[string][]ResolvedUpstream{}, APIKeys: map[string]*model.APIKey{},
		General: settings.DefaultGeneral(), Failover: settings.DefaultFailover(),
		Output: settings.DefaultOutputFilter(), Security: settings.DefaultSecurity(),
		Alerts: settings.DefaultQuotaAlerts(),
	}
}

func (m *Manager) Get() *Snapshot { return m.cur.Load() }

// Trigger 非阻塞投递一次重载事件（实时触发，REQ-004 方式一）。
func (m *Manager) Trigger() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Bump 在配置写入事务提交后递增 DB config_meta.counter 并即时触发本实例重载。
// 其他实例通过轮询发现 counter 变化后自行重载，满足多副本部署。
func (m *Manager) Bump() {
	var meta settings.ConfigMeta
	if err := settings.LoadKV(m.db, model.SetKeyConfigMeta, &meta, settings.ConfigMeta{}); err == nil {
		meta.Counter++
		_ = settings.SaveKV(m.db, model.SetKeyConfigMeta, meta)
	}
	m.Trigger()
}

func (m *Manager) currentMeta() settings.ConfigMeta {
	var meta settings.ConfigMeta
	_ = settings.LoadKV(m.db, model.SetKeyConfigMeta, &meta, settings.ConfigMeta{})
	return meta
}

// Start 启动重载循环：trigger 即时 + 轮询兜底。
func (m *Manager) Start(ctx context.Context) {
	if err := m.Reload("startup"); err != nil {
		m.setError(err.Error()) // 首载失败不阻断服务（NFR-005）
	}
	go func() {
		for {
			interval := m.pollInterval()
			ticker := time.NewTicker(interval)
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-m.trigger:
				ticker.Stop()
				_, _ = m.ReloadIfChanged("event")
			case <-ticker.C:
				ticker.Stop()
				_, _ = m.ReloadIfChanged("polling")
			}
		}
	}()
}

func (m *Manager) pollInterval() time.Duration {
	s := m.Get().General.HotReloadSeconds
	if s < 1 {
		s = 3
	}
	return time.Duration(s) * time.Second
}

// ReloadIfChanged 仅当 DB counter 变化时重载。
func (m *Manager) ReloadIfChanged(reason string) (bool, error) {
	meta := m.currentMeta()
	m.mu.Lock()
	changed := meta.Counter != m.dbCount || m.cur.Load().Version == "-"
	m.mu.Unlock()
	if !changed {
		return false, nil
	}
	return true, m.Reload(reason)
}

// Reload 全量加载并原子替换快照；无论成败都更新版本状态供 UI 展示。
func (m *Manager) Reload(reason string) error {
	if !m.busy.CompareAndSwap(false, true) {
		return nil // 已有重载在途
	}
	defer m.busy.Store(false)

	snap := emptySnapshot()
	snap.LoadedAt = time.Now()
	var loadErrs []string

	// --- providers（解密进内存明文副本） ---
	var providers []model.Provider
	if err := m.db.Find(&providers).Error; err != nil {
		loadErrs = append(loadErrs, "providers: "+err.Error())
	}
	for i := range providers {
		p := providers[i]
		if plain, err := m.enc.Decrypt(p.APIKeyEnc); err == nil {
			p.APIKey = plain
			p.APIKeyEnc = ""
		} else {
			p.APIKey = "" // 解密失败：保留空密钥，该上游调用时会失败
		}
		snap.Providers[p.ID] = &p
		snap.ProviderByName[p.Name] = &p
	}

	// --- 模型别名 → 解析上游 ---
	var aliases []model.ModelAlias
	if err := m.db.Preload("Upstreams").Find(&aliases).Error; err != nil {
		loadErrs = append(loadErrs, "model_aliases: "+err.Error())
	}
	for _, a := range aliases {
		if !a.Enabled {
			continue
		}
		var ups []ResolvedUpstream
		for _, u := range a.Upstreams {
			p, ok := snap.Providers[u.ProviderID]
			if !ok || !p.Enabled {
				continue
			}
			w := u.Weight
			if w <= 0 {
				w = 1
			}
			ups = append(ups, ResolvedUpstream{ProviderID: p.ID, ProviderName: p.Name,
				Protocol: p.Protocol, BaseURL: p.BaseURL, APIKey: p.APIKey,
				UpstreamModel: u.UpstreamModel, Weight: w})
		}
		if len(ups) > 0 {
			snap.Aliases[a.Alias] = ups
		}
	}

	// --- 网关 API Key ---
	var keys []model.APIKey
	if err := m.db.Find(&keys).Error; err != nil {
		loadErrs = append(loadErrs, "apikeys: "+err.Error())
	}
	for i := range keys {
		k := keys[i]
		snap.APIKeys[k.KeyHash] = &k
	}

	// --- 护栏规则编译 ---
	var kws []model.GuardKeyword
	if err := m.db.Where("enabled = ?", true).Find(&kws).Error; err != nil {
		loadErrs = append(loadErrs, "keywords: "+err.Error())
	}
	for _, kw := range kws {
		c := CompiledKeyword{Raw: kw}
		if kw.MatchMode == "regex" {
			re, err := regexp.Compile("(?i)" + kw.Word)
			if err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("keyword#%d 非法正则: %v", kw.ID, err))
				continue
			}
			c.Re = re
		}
		snap.Keywords = append(snap.Keywords, c)
	}

	var piis []model.PIIRule
	if err := m.db.Where("enabled = ?", true).Find(&piis).Error; err != nil {
		loadErrs = append(loadErrs, "pii_rules: "+err.Error())
	}
	for _, r := range piis {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("pii#%d 非法正则: %v", r.ID, err))
			continue
		}
		snap.PIIRules = append(snap.PIIRules, CompiledPII{Raw: r, Re: re})
	}

	var injs []model.InjectionRule
	if err := m.db.Where("enabled = ?", true).Find(&injs).Error; err != nil {
		loadErrs = append(loadErrs, "injection_rules: "+err.Error())
	}
	for _, r := range injs {
		c := CompiledInjection{Raw: r}
		if r.MatchMode == "regex" {
			re, err := regexp.Compile("(?i)" + r.Pattern)
			if err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("injection#%d 非法正则: %v", r.ID, err))
				continue
			}
			c.Re = re
		}
		snap.Injection = append(snap.Injection, c)
	}

	// --- 配额与限流 ---
	var quotas []model.Quota
	if err := m.db.Where("enabled = ?", true).Find(&quotas).Error; err != nil {
		loadErrs = append(loadErrs, "quotas: "+err.Error())
	}
	snap.Quotas = quotas
	var rls []model.RateLimitRule
	if err := m.db.Where("enabled = ?", true).Find(&rls).Error; err != nil {
		loadErrs = append(loadErrs, "rate_limits: "+err.Error())
	}
	snap.RateLimits = rls

	// --- KV 设置 ---
	_ = settings.LoadKV(m.db, model.SetKeyGeneral, &snap.General, settings.DefaultGeneral())
	_ = settings.LoadKV(m.db, model.SetKeyFailover, &snap.Failover, settings.DefaultFailover())
	_ = settings.LoadKV(m.db, model.SetKeyOutputFilt, &snap.Output, settings.DefaultOutputFilter())
	_ = settings.LoadKV(m.db, model.SetKeySecurity, &snap.Security, settings.DefaultSecurity())
	_ = settings.LoadKV(m.db, model.SetKeyQuotaAlert, &snap.Alerts, settings.DefaultQuotaAlerts())

	snap.Counts = map[string]int{
		"providers": len(snap.Providers), "model_aliases": len(snap.Aliases),
		"keywords": len(snap.Keywords), "pii_rules": len(snap.PIIRules),
		"injection_rules": len(snap.Injection), "quotas": len(snap.Quotas),
		"rate_limits": len(snap.RateLimits), "apikeys": len(snap.APIKeys),
	}

	status, errMsg := "ok", ""
	if len(loadErrs) > 0 {
		status = "error"
		errMsg = fmt.Sprintf("%d 个模块加载异常: %v", len(loadErrs), loadErrs)
	}
	// 规则编译错误不阻断替换：旧快照中正常部分已被新快照覆盖，异常模块记录可见。
	snap.Version = time.Now().Format("v20060102.150405")
	snap.LoadErrors = loadErrs

	meta := m.currentMeta()
	m.mu.Lock()
	m.dbCount = meta.Counter
	m.Version = snap.Version
	m.Status = status
	m.Err = errMsg
	m.mu.Unlock()

	m.cur.Store(snap) // ★ 原子替换：进行中请求仍持旧快照，新请求用新配置
	m.recordVersion(snap, status, errMsg, reason)
	return nil
}

func (m *Manager) setError(msg string) {
	m.mu.Lock()
	m.Status = "error"
	m.Err = msg
	m.mu.Unlock()
	var meta settings.ConfigMeta
	_ = settings.LoadKV(m.db, model.SetKeyConfigMeta, &meta, settings.ConfigMeta{})
	meta.Status = "error"
	meta.ErrMessage = msg
	_ = settings.SaveKV(m.db, model.SetKeyConfigMeta, meta)
}

// recordVersion 写 ConfigVersion（含全量快照 JSON）+ 逐模块加载日志（REQ-004A）。
func (m *Manager) recordVersion(snap *Snapshot, status, errMsg, reason string) {
	bundle := m.exportBundleFrom(snap)
	sj := ""
	if b, err := json.Marshal(bundle); err == nil {
		sj = string(b)
	}
	ver := model.ConfigVersion{Version: snap.Version, SnapshotJSON: sj, Status: status,
		Message: fmt.Sprintf("[%s] %s", reason, errMsg)}
	m.db.Create(&ver)
	var drop []uint
	m.db.Model(&model.ConfigVersion{}).Order("id desc").Offset(50).Limit(500).Pluck("id", &drop)
	if len(drop) > 0 {
		m.db.Delete(&model.ConfigVersion{}, "id IN ?", drop)
	}

	m.logLoad("providers", "success", fmt.Sprintf("供应商配置加载完成 (%d项)", snap.Counts["providers"]))
	m.logLoad("model_aliases", "success", fmt.Sprintf("模型别名配置加载完成 (%d项)", snap.Counts["model_aliases"]))
	m.logLoad("keywords", "success", fmt.Sprintf("敏感词库加载完成 (%d项)", snap.Counts["keywords"]))
	m.logLoad("quotas", "success", fmt.Sprintf("配额规则加载完成 (%d项)", snap.Counts["quotas"]))
	if status != "ok" {
		m.logLoad("all", "error", errMsg)
	}

	var meta settings.ConfigMeta
	_ = settings.LoadKV(m.db, model.SetKeyConfigMeta, &meta, settings.ConfigMeta{})
	meta.Version = snap.Version
	meta.Status = status
	meta.ErrMessage = errMsg
	_ = settings.SaveKV(m.db, model.SetKeyConfigMeta, meta)
}

func (m *Manager) logLoad(module, status, msg string) {
	m.db.Create(&model.ConfigLoadLog{Time: time.Now(), Module: module, Status: status, Message: msg})
	var drop []uint
	m.db.Model(&model.ConfigLoadLog{}).Order("id desc").Offset(200).Limit(500).Pluck("id", &drop)
	if len(drop) > 0 {
		m.db.Delete(&model.ConfigLoadLog{}, "id IN ?", drop)
	}
}

// StatusInfo 供配置状态页展示（REQ-004A）。
type StatusInfo struct {
	Version      string         `json:"version"`
	LastLoadedAt time.Time      `json:"last_loaded_at"`
	Status       string         `json:"status"`
	ErrMessage   string         `json:"error_message"`
	Modules      map[string]int `json:"modules"`
}

func (m *Manager) Info() StatusInfo {
	s := m.Get()
	return StatusInfo{Version: s.Version, LastLoadedAt: s.LoadedAt,
		Status: m.Status, ErrMessage: m.Err, Modules: s.Counts}
}
