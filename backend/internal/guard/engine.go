// Package guard 实现护栏引擎（REQ-008~011 / PRD 6.1 核心业务层）。
// 输入检测：敏感词 + PII + 注入规则；输出检测：敏感词 + PII + 违规策略。
// NFR-005：引擎内部 panic 会被吞掉并按"通过"处理，不阻断业务。
package guard

import (
	"encoding/json"
	"fmt"
	"strings"

	"llmrouter/internal/runtime"
)

type Finding struct {
	Type     string `json:"type"` // keyword|pii|injection
	Value    string `json:"value"`
	Category string `json:"category"`
	Action   string `json:"action"`
}

type Verdict struct {
	Blocked     bool      `json:"blocked"`
	BlockReason string    `json:"block_reason,omitempty"`
	Findings    []Finding `json:"findings"`
	Text        string    `json:"-"` // 处理后的文本（PII 已按规则脱敏）
}

func (v Verdict) FindingsJSON() string {
	if len(v.Findings) == 0 {
		return ""
	}
	b, _ := json.Marshal(v.Findings)
	return string(b)
}

// FindingsJSON 将 findings 列表序列化为 JSON 字符串（空列表返回空串）。
func FindingsJSON(fs []Finding) string {
	if len(fs) == 0 {
		return ""
	}
	b, _ := json.Marshal(fs)
	return string(b)
}

// FindingsJSONRaw 将已有的 findings JSON 字符串解析后追加新 findings，再序列化。
// 解析失败时丢弃旧串，仅序列化新 findings。
func FindingsJSONRaw(existing string, fs []Finding) string {
	var merged []Finding
	if existing != "" {
		_ = json.Unmarshal([]byte(existing), &merged)
	}
	merged = append(merged, fs...)
	if len(merged) == 0 {
		return ""
	}
	b, _ := json.Marshal(merged)
	return string(b)
}

// CheckInput 输入双向过滤：命中 block 级规则即拦截；mask 级 PII 就地替换。
func CheckInput(snap *runtime.Snapshot, text string) (vd Verdict) {
	vd.Text = text
	if !snap.General.GuardEnabled || strings.TrimSpace(text) == "" {
		return vd
	}
	defer func() {
		if r := recover(); r != nil { // 护栏故障降级（NFR-005）
			vd = Verdict{Text: text}
		}
	}()

	lower := strings.ToLower(text) // M-26：整段文本只小写一次（旧实现每条规则各扫一遍全文）

	// 敏感词
	for _, kw := range snap.Keywords {
		if !matchKeyword(kw, text, lower) {
			continue
		}
		f := Finding{Type: "keyword", Value: display(kw.Raw.Word), Category: "keyword." + kw.Raw.Category, Action: kw.Raw.Action}
		vd.Findings = append(vd.Findings, f)
		if kw.Raw.Action == "block" && !vd.Blocked {
			vd.Blocked = true
			vd.BlockReason = fmt.Sprintf("命中敏感词策略 [%s/%s]", kw.Raw.Category, f.Value)
		}
	}

	// PII：mask 动作就地替换，block 动作拦截
	for _, r := range snap.PIIRules {
		loc := r.Re.FindAllStringIndex(text, -1)
		if len(loc) == 0 {
			continue
		}
		repl := r.Raw.Replacement
		if repl == "" {
			repl = "[REDACTED]"
		}
		vd.Findings = append(vd.Findings, Finding{Type: "pii",
			Value: fmt.Sprintf("%s x%d", r.Raw.Name, len(loc)), Category: "pii." + r.Raw.Category, Action: r.Raw.Action})
		switch r.Raw.Action {
		case "mask":
			vd.Text = r.Re.ReplaceAllLiteralString(vd.Text, repl) // M-28：替换串按字面处理（旧版会把 "$1" 展开为捕获组，可能泄出原文）
		case "block":
			if !vd.Blocked {
				vd.Blocked = true
				vd.BlockReason = fmt.Sprintf("命中 PII 拦截规则 [%s]", r.Raw.Name)
			}
		}
	}

	// 注入规则
	for _, inj := range snap.Injection {
		if !matchInjection(inj, text, lower) {
			continue
		}
		vd.Findings = append(vd.Findings, Finding{Type: "injection", Value: inj.Raw.Name,
			Category: "injection", Action: inj.Raw.Action})
		if inj.Raw.Action == "block" && !vd.Blocked {
			vd.Blocked = true
			vd.BlockReason = fmt.Sprintf("疑似提示词注入/越狱攻击 [%s]", inj.Raw.Name)
		}
	}
	return vd
}

// CheckOutput 输出过滤（REQ-011）：对上游返回内容再次检测。
// violation_strategy: block=报错终止 / replace=替换为安全提示语 / log=仅记录。
func CheckOutput(snap *runtime.Snapshot, text string) (vd Verdict) {
	vd = Verdict{Text: text}
	if !snap.General.GuardEnabled || !snap.Output.Enabled || strings.TrimSpace(text) == "" {
		return vd
	}
	defer func() {
		if r := recover(); r != nil {
			vd = Verdict{Text: text}
		}
	}()

	lower := strings.ToLower(text) // M-26：单次小写
	for _, kw := range snap.Keywords {
		if matchKeyword(kw, text, lower) && kw.Raw.Action == "block" {
			vd.Blocked = true
			vd.BlockReason = fmt.Sprintf("输出命中敏感词策略 [%s]", display(kw.Raw.Word))
			vd.Findings = append(vd.Findings, Finding{Type: "keyword", Value: display(kw.Raw.Word),
				Category: "output.keyword." + kw.Raw.Category, Action: "block"})
			break // 拦截原因取首条命中即可，无需扫完全部词表
		}
	}
	for _, r := range snap.PIIRules {
		if r.Raw.Action == "mask" && r.Re.MatchString(text) {
			repl := r.Raw.Replacement
			if repl == "" {
				repl = "[REDACTED]"
			}
			vd.Text = r.Re.ReplaceAllLiteralString(vd.Text, repl) // M-28：替换串按字面处理（旧版会把 "$1" 展开为捕获组，可能泄出原文）
			vd.Findings = append(vd.Findings, Finding{Type: "pii", Value: r.Raw.Name, Category: "output.pii." + r.Raw.Category, Action: "mask"})
		}
	}
	return vd
}

// ApplyOutputStrategy 按策略生成最终对客文本 / 拦截决定。
// 返回 (finalText, blocked)。
func ApplyOutputStrategy(snap *runtime.Snapshot, vd Verdict) (string, bool) {
	if !vd.Blocked {
		return vd.Text, false
	}
	switch snap.Output.ViolationStrategy {
	case "block":
		return "", true
	case "log":
		return vd.Text, false
	default: // replace
		msg := snap.Output.SafeMessage
		if msg == "" {
			msg = "抱歉，该回答包含不当内容。"
		}
		return msg, false
	}
}

// MaskForLog 审计日志脱敏（NFR-009）：所有 mask 类 PII 规则替换 + 截断到 4000 字节。
func MaskForLog(snap *runtime.Snapshot, text string) string {
	return MaskForLogLimit(snap, text, 4000)
}

// MaskForLogLimit 同 MaskForLog，但截断上限可调。思维链（reasoning_content）
// 走 5 万字节上限以基本完整留存排查价值；最终回答仍走默认 4000。
// limit<=0 表示不截断（仅脱敏）。
func MaskForLogLimit(snap *runtime.Snapshot, text string, limit int) string {
	out := text
	for _, r := range snap.PIIRules {
		if r.Re != nil {
			repl := r.Raw.Replacement
			if repl == "" {
				repl = "[REDACTED]"
			}
			out = r.Re.ReplaceAllString(out, repl)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit] + "...[truncated]"
	}
	return out
}

// ---- 匹配原语 ----

// matchKeyword lower 为 text 的单次小写结果（M-26），contains 模式复用。
func matchKeyword(kw runtime.CompiledKeyword, text, lower string) bool {
	switch kw.Raw.MatchMode {
	case "regex":
		return kw.Re != nil && kw.Re.MatchString(text)
	case "exact":
		return strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(kw.Raw.Word))
	default: // contains
		return strings.Contains(lower, strings.ToLower(kw.Raw.Word))
	}
}

func matchInjection(inj runtime.CompiledInjection, text, lower string) bool {
	if inj.Re != nil {
		return inj.Re.MatchString(text)
	}
	return strings.Contains(lower, strings.ToLower(inj.Raw.Pattern))
}

func display(word string) string {
	r := []rune(word)
	if len(r) > 12 {
		return string(r[:12]) + "…"
	}
	return word
}
