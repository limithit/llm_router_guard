// guard_fields.go 输入护栏「全字段」引擎（SEC-10）：
// 旧实现仅扫描 Messages[i].Content —— 工具调用参数（arguments）、工具名、工具结果名、
// 工具定义 description/parameters 全部绕过关键词/PII/注入检测，等于给注入攻击留了一条
// 结构化旁路（把提示词塞进 tool arguments 即免检）。
//
// 本文件对「将转发上游的每一个文本字段」执行同一套检测与就地改写：
//   - 纯文本字段（消息内容、name、description）：CheckInput 直接替换；
//   - JSON 结构字段（arguments、parameters）：递归遍历其中的字符串值逐个脱敏后回序列化，
//     保持 JSON 结构合法（不能把整段 JSON 当纯文本打码，否则上游 400）；
//     非 JSON 的 arguments（个别客户端传纯文本）退化为纯文本检测。
//
// 任一字段命中 block 级规则 → 整个请求拦截（对客仅回通用文案，细节走审计，M-13）。
package gateway

import (
	"encoding/json"
	"strings"

	"llmrouter/internal/adapter"
	"llmrouter/internal/guard"
	"llmrouter/internal/runtime"
)

type fieldGuard struct {
	findings    []guard.Finding
	blockReason string
	blockCat    string
}

func (fg *fieldGuard) take(vd guard.Verdict) {
	fg.findings = append(fg.findings, vd.Findings...)
	if vd.Blocked && fg.blockReason == "" {
		fg.blockReason = vd.BlockReason
		for _, f := range vd.Findings {
			if f.Action == "block" {
				fg.blockCat = f.Category
				break
			}
		}
	}
}

// guardCanonicalInput 扫描并就地改写请求的全部文本字段。返回 findings 与首个拦截原因/类别。
func guardCanonicalInput(snap *runtime.Snapshot, cr *adapter.CanonicalRequest) (findings []guard.Finding, blockReason, blockCat string) {
	fg := &fieldGuard{}
	for i := range cr.Messages {
		m := &cr.Messages[i]
		if m.Content != "" {
			vd := guard.CheckInput(snap, m.Content)
			m.Content = vd.Text
			fg.take(vd)
		}
		if m.Name != "" { // tool 结果消息的函数名
			vd := guard.CheckInput(snap, m.Name)
			m.Name = vd.Text
			fg.take(vd)
		}
		for j := range m.ToolCalls {
			tc := &m.ToolCalls[j]
			if tc.Name != "" {
				vd := guard.CheckInput(snap, tc.Name)
				tc.Name = vd.Text
				fg.take(vd)
			}
			if tc.Arguments != "" {
				out, vd := guardJSONLike(snap, tc.Arguments)
				tc.Arguments = out
				fg.take(vd)
			}
		}
	}
	for i := range cr.Tools {
		t := &cr.Tools[i]
		if t.Name != "" {
			vd := guard.CheckInput(snap, t.Name)
			t.Name = vd.Text
			fg.take(vd)
		}
		if t.Description != "" {
			vd := guard.CheckInput(snap, t.Description)
			t.Description = vd.Text
			fg.take(vd)
		}
		if len(t.Parameters) > 0 {
			out, vd := guardJSONLike(snap, string(t.Parameters))
			t.Parameters = json.RawMessage(out)
			fg.take(vd)
		}
	}
	return fg.findings, fg.blockReason, fg.blockCat
}

// guardJSONLike 对 JSON 文档内的全部字符串值跑 CheckInput（就地脱敏，结构保留）；
// 非 JSON 文档按纯文本处理。
func guardJSONLike(snap *runtime.Snapshot, raw string) (string, guard.Verdict) {
	return guardJSONLikeWith(snap, raw, guard.CheckInput)
}

// guardJSONLikeWith 同上，可指定检测函数（输出护栏用 CheckOutput）。
func guardJSONLikeWith(snap *runtime.Snapshot, raw string, check func(*runtime.Snapshot, string) guard.Verdict) (string, guard.Verdict) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		vd := check(snap, raw)
		return vd.Text, vd
	}
	merged := guard.Verdict{Text: raw}
	walkJSONStrings(v, func(s string) string {
		vd := check(snap, s)
		merged.Findings = append(merged.Findings, vd.Findings...)
		if vd.Blocked && merged.BlockReason == "" {
			merged.Blocked = true
			merged.BlockReason = vd.BlockReason
		}
		return vd.Text
	})
	b, err := json.Marshal(v)
	if err != nil { // 理论不可达（仅改 string 值）；兜底原样返回
		return raw, merged
	}
	return string(b), merged
}

// walkJSONStrings 深度遍历 JSON 值，对每个 string 应用 fn 并原地替换。
func walkJSONStrings(v any, fn func(string) string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch x := val.(type) {
			case string:
				t[k] = fn(x)
			case map[string]any, []any:
				walkJSONStrings(x, fn)
			}
		}
	case []any:
		for i, val := range t {
			switch x := val.(type) {
			case string:
				t[i] = fn(x)
			case map[string]any, []any:
				walkJSONStrings(x, fn)
			}
		}
	}
}

// mergeToolText 输出工具调用的规范化文本形态（审计与输出护栏共用）。
// 注意：arguments 先解析再紧凑序列化——防上游把含 "<tool_call" 字样的字符串参数
// 拼进审计/检测文本后伪造边界标签（伪标签注入）。
func mergeToolText(tcs []adapter.CanonicalToolCall) string {
	if len(tcs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, tc := range tcs {
		args := tc.Arguments
		var v any
		if json.Unmarshal([]byte(args), &v) == nil {
			if compact, err := json.Marshal(v); err == nil {
				args = string(compact)
			}
		}
		encName, _ := json.Marshal(tc.Name)
		encArgs, _ := json.Marshal(args)
		sb.WriteString("<tool_call>{\"name\":")
		sb.Write(encName)
		sb.WriteString(",\"arguments\":")
		sb.Write(encArgs)
		sb.WriteString("}</tool_call> ")
	}
	return sb.String()
}
