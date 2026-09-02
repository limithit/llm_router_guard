// bundle.go 全量配置包：用于版本快照、一键回滚（REQ-004A）、
// 配置导出与数据备份恢复（REQ-019）。
package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"llmrouter/internal/model"
)

// ConfigBundle 可序列化的全量配置（密钥保持 DB 原样密文，跨版本/备份一致）。
type ConfigBundle struct {
	Providers   []model.Provider      `json:"providers"`
	Aliases     []model.ModelAlias    `json:"aliases"`
	Keywords    []model.GuardKeyword  `json:"keywords"`
	PIIRules    []model.PIIRule       `json:"pii_rules"`
	Injection   []model.InjectionRule `json:"injection_rules"`
	Quotas      []model.Quota         `json:"quotas"`
	RateLimits  []model.RateLimitRule `json:"rate_limits"`
	APIKeys     []model.APIKey        `json:"api_keys"`
	Settings    map[string]string     `json:"settings"`
	ExportedAt  time.Time             `json:"exported_at"`
}

// ExportBundle 从数据库导出全量配置。
func (m *Manager) ExportBundle() (*ConfigBundle, error) {
	b := &ConfigBundle{Settings: map[string]string{}, ExportedAt: time.Now()}
	if err := m.db.Find(&b.Providers).Error; err != nil {
		return nil, err
	}
	if err := m.db.Preload("Upstreams").Find(&b.Aliases).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.Keywords).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.PIIRules).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.Injection).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.Quotas).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.RateLimits).Error; err != nil {
		return nil, err
	}
	if err := m.db.Find(&b.APIKeys).Error; err != nil {
		return nil, err
	}
	var sets []model.SystemSetting
	if err := m.db.Find(&sets).Error; err != nil {
		return nil, err
	}
	for _, s := range sets {
		if s.Key == model.SetKeyConfigMeta { // 恢复时保留新的 meta，不覆盖
			continue
		}
		b.Settings[s.Key] = s.ValueJSON
	}
	return b, nil
}

// exportBundleFrom 版本快照记录用（忽略快照参数，统一以 DB 为准）。
func (m *Manager) exportBundleFrom(_ *Snapshot) *ConfigBundle {
	b, err := m.ExportBundle()
	if err != nil {
		return &ConfigBundle{Settings: map[string]string{}, ExportedAt: time.Now()}
	}
	return b
}

// RestoreBundle 事务内清空并回写全部配置表（REQ-004A 回滚 / REQ-019 恢复共用）。
func (m *Manager) RestoreBundle(b *ConfigBundle) error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{
			&model.Provider{}, &model.AliasUpstream{}, &model.ModelAlias{},
			&model.GuardKeyword{}, &model.PIIRule{}, &model.InjectionRule{},
			&model.Quota{}, &model.RateLimitRule{},
		} {
			if err := tx.Where("1 = 1").Delete(table).Error; err != nil {
				return fmt.Errorf("clear %T: %w", table, err)
			}
		}
		if len(b.Providers) > 0 {
			if err := tx.Create(&b.Providers).Error; err != nil {
				return err
			}
		}
		for _, a := range b.Aliases {
			up := a.Upstreams
			a.Upstreams = nil
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
			for i := range up {
				up[i].ID = 0
				up[i].AliasID = a.ID
			}
			if len(up) > 0 {
				if err := tx.Create(&up).Error; err != nil {
					return err
				}
			}
		}
		if len(b.Keywords) > 0 {
			if err := tx.Create(&b.Keywords).Error; err != nil {
				return err
			}
		}
		if len(b.PIIRules) > 0 {
			if err := tx.Create(&b.PIIRules).Error; err != nil {
				return err
			}
		}
		if len(b.Injection) > 0 {
			if err := tx.Create(&b.Injection).Error; err != nil {
				return err
			}
		}
		if len(b.Quotas) > 0 {
			if err := tx.Create(&b.Quotas).Error; err != nil {
				return err
			}
		}
		if len(b.RateLimits) > 0 {
			if err := tx.Create(&b.RateLimits).Error; err != nil {
				return err
			}
		}
		for k, v := range b.Settings {
			if err := tx.Save(&model.SystemSetting{Key: k, ValueJSON: v}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Rollback 回滚到上一个 ok 版本（REQ-004A ⑤）。
func (m *Manager) Rollback() (string, error) {
	var vers []model.ConfigVersion
	if err := m.db.Where("status = ?", "ok").Order("id desc").Limit(2).Find(&vers).Error; err != nil {
		return "", err
	}
	if len(vers) < 2 {
		return "", fmt.Errorf("没有可回滚的历史版本")
	}
	target := vers[1] // 上一版本
	var b ConfigBundle
	if err := json.Unmarshal([]byte(target.SnapshotJSON), &b); err != nil {
		return "", fmt.Errorf("版本快照损坏: %w", err)
	}
	if b.Settings == nil {
		b.Settings = map[string]string{}
	}
	if err := m.RestoreBundle(&b); err != nil {
		return "", err
	}
	m.Bump()
	_ = m.Reload("rollback")
	return target.Version, nil
}

// LoadBundleJSON 从备份 JSON 文本解析配置包。
func LoadBundleJSON(data string) (*ConfigBundle, error) {
	var b ConfigBundle
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		return nil, fmt.Errorf("备份文件解析失败: %w", err)
	}
	if b.Settings == nil {
		b.Settings = map[string]string{}
	}
	return &b, nil
}
