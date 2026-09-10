// Package adapter 多协议适配层（REQ-017 / PRD 6.1）：
// 三种入站协议（OpenAI Chat Completions / OpenAI Responses / Anthropic Messages）
// 统一转换为规范模型（Canonical Model），再按上游供应商协议序列化。
// 新增协议仅需在此包扩展解析器与序列化器（NFR-011）。
package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Protocol = string

const (
	ProtoOpenAIChat      = "openai_chat"
	ProtoOpenAIResponses = "openai_responses"
	ProtoAnthropic       = "anthropic"
)

// ---- 协议无关工具调用模型 ----
// 三种协议的工具调用形态互不相同（openai 字符串分片 arguments / anthropic JSON 对象 +
// input_json_delta / responses function_call 项），必须经规范结构互转。

// CanonicalTool 函数定义（Parameters 为 JSON Schema）。
type CanonicalTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// CanonicalToolChoice 工具选择策略：auto / none / required / tool（Name 指定函数）。
type CanonicalToolChoice struct {
	Mode string `json:"mode"`
	Name string `json:"name,omitempty"`
}

// CanonicalToolCall 一次工具调用（Arguments 统一为 JSON 字符串形态）。
type CanonicalToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCallDelta 流式工具调用增量（ID/Name 仅首片携带，ArgsDelta 跨片拼接）。
// ToolIndex 是上游侧的分片序号（openai tool index / anthropic block index）。
type ToolCallDelta struct {
	ToolIndex int
	ID        string
	Name      string
	ArgsDelta string
}

type Message struct {
	Role       string              `json:"role"`
	Content    string              `json:"content"`
	ToolCallID string              `json:"tool_call_id,omitempty"` // role=tool 的结果消息关联 ID
	Name       string              `json:"name,omitempty"`         // tool 消息的函数名
	ToolCalls  []CanonicalToolCall `json:"tool_calls,omitempty"`   // assistant 历史中的工具调用
}

type CanonicalRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	// Tools/ToolChoice 工具调用定义（agent 场景必需：丢弃后模型会把工具调用当纯文本输出，
	// agent turn 直接中断）。协议无关，序列化时按上游协议转换。
	Tools      []CanonicalTool      `json:"-"`
	ToolChoice *CanonicalToolChoice `json:"-"`
}

type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
}

type CanonicalResponse struct {
	ID           string              `json:"id"`
	Model        string              `json:"model"`
	Content      string              `json:"content"`
	Reasoning    string              `json:"reasoning"`            // 推理模型思维链（GLM/DeepSeek reasoning_content）
	ToolCalls    []CanonicalToolCall `json:"tool_calls,omitempty"` // 非流式工具调用（协议无关）
	FinishReason string              `json:"finish_reason"`
	Usage        Usage               `json:"usage"`
}

// ---- 入站解析 ----

func ParseRequest(p Protocol, body []byte) (*CanonicalRequest, error) {
	switch p {
	case ProtoOpenAIChat:
		return parseOpenAIChat(body)
	case ProtoOpenAIResponses:
		return parseResponses(body)
	case ProtoAnthropic:
		return parseAnthropic(body)
	}
	return nil, fmt.Errorf("unsupported protocol %q", p)
}

func parseOpenAIChat(body []byte) (*CanonicalRequest, error) {
	var raw struct {
		Model       string          `json:"model"`
		Messages    json.RawMessage `json:"messages"`
		Stream      bool            `json:"stream"`
		MaxTokens   int             `json:"max_tokens"`
		Temperature *float64        `json:"temperature"`
		TopP        *float64        `json:"top_p"`
		Tools       json.RawMessage `json:"tools"`
		ToolChoice  json.RawMessage `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	var msgs []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		Name       string          `json:"name"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw.Messages, &msgs); err != nil {
		return nil, fmt.Errorf("invalid messages: %w", err)
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
	if len(raw.Tools) > 0 {
		var ots []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(raw.Tools, &ots) == nil {
			for _, t := range ots {
				if t.Type == "function" && t.Function.Name != "" {
					cr.Tools = append(cr.Tools, CanonicalTool{Name: t.Function.Name,
						Description: t.Function.Description, Parameters: t.Function.Parameters})
				}
			}
		}
	}
	cr.ToolChoice = parseOpenAIToolChoice(raw.ToolChoice)
	for _, m := range msgs {
		cm := Message{Role: m.Role, Content: extractContentText(m.Content),
			ToolCallID: m.ToolCallID, Name: m.Name}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, CanonicalToolCall{ID: tc.ID, Name: tc.Function.Name,
				Arguments: tc.Function.Arguments})
		}
		cr.Messages = append(cr.Messages, cm)
	}
	return cr, nil
}

// parseOpenAIToolChoice openai tool_choice（字符串或 {type,function:{name}} 对象）→ 规范。
func parseOpenAIToolChoice(raw json.RawMessage) *CanonicalToolChoice {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "auto", "none", "required":
			return &CanonicalToolChoice{Mode: s}
		}
		return nil
	}
	var o struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &o) == nil && o.Type == "function" && o.Function.Name != "" {
		return &CanonicalToolChoice{Mode: "tool", Name: o.Function.Name}
	}
	return nil
}

func parseResponses(body []byte) (*CanonicalRequest, error) {
	var raw struct {
		Model        string          `json:"model"`
		Input        json.RawMessage `json:"input"`
		Instructions string          `json:"instructions"`
		Stream       bool            `json:"stream"`
		MaxTokens    int             `json:"max_output_tokens"`
		Temperature  *float64        `json:"temperature"`
		TopP         *float64        `json:"top_p"`
		Tools        json.RawMessage `json:"tools"`
		ToolChoice   json.RawMessage `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
	if len(raw.Tools) > 0 {
		// responses 协议 tools 为扁平结构（function 字段不嵌套）
		var rts []struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		if json.Unmarshal(raw.Tools, &rts) == nil {
			for _, t := range rts {
				if t.Type == "function" && t.Name != "" {
					cr.Tools = append(cr.Tools, CanonicalTool{Name: t.Name,
						Description: t.Description, Parameters: t.Parameters})
				}
			}
		}
	}
	// responses tool_choice: 字符串或 {type:"function", name}（不嵌套 function）
	if len(raw.ToolChoice) > 0 {
		var s string
		if json.Unmarshal(raw.ToolChoice, &s) == nil {
			switch s {
			case "auto", "none", "required":
				cr.ToolChoice = &CanonicalToolChoice{Mode: s}
			}
		} else {
			var o struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw.ToolChoice, &o) == nil && o.Type == "function" && o.Name != "" {
				cr.ToolChoice = &CanonicalToolChoice{Mode: "tool", Name: o.Name}
			}
		}
	}
	if raw.Instructions != "" {
		cr.Messages = append(cr.Messages, Message{Role: "system", Content: raw.Instructions})
	}
	// input: string 或 item 数组
	var s string
	if err := json.Unmarshal(raw.Input, &s); err == nil {
		cr.Messages = append(cr.Messages, Message{Role: "user", Content: s})
		return cr, nil
	}
	var items []struct {
		Role    string          `json:"role"`
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
		// function_call（assistant 的历史工具调用）
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		// function_call_output（工具结果）
		Output string `json:"output"`
	}
	if err := json.Unmarshal(raw.Input, &items); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	for _, it := range items {
		switch it.Type {
		case "function_call":
			if it.Name == "" {
				continue
			}
			cr.Messages = append(cr.Messages, Message{Role: "assistant",
				ToolCalls: []CanonicalToolCall{{ID: it.CallID, Name: it.Name, Arguments: it.Arguments}}})
		case "function_call_output":
			cr.Messages = append(cr.Messages, Message{Role: "tool", ToolCallID: it.CallID, Content: it.Output})
		default:
			if it.Role == "" {
				continue
			}
			cr.Messages = append(cr.Messages, Message{Role: it.Role, Content: extractContentText(it.Content)})
		}
	}
	if len(cr.Messages) == 0 {
		return nil, fmt.Errorf("input has no message items")
	}
	return cr, nil
}

func parseAnthropic(body []byte) (*CanonicalRequest, error) {
	var raw struct {
		Model    string          `json:"model"`
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Stream      bool            `json:"stream"`
		MaxTokens   int             `json:"max_tokens"`
		Temperature *float64        `json:"temperature"`
		TopP        *float64        `json:"top_p"`
		Tools       json.RawMessage `json:"tools"`
		ToolChoice  json.RawMessage `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
	if len(raw.Tools) > 0 {
		var ats []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}
		if json.Unmarshal(raw.Tools, &ats) == nil {
			for _, t := range ats {
				if t.Name != "" {
					cr.Tools = append(cr.Tools, CanonicalTool{Name: t.Name,
						Description: t.Description, Parameters: t.InputSchema})
				}
			}
		}
	}
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw.ToolChoice, &tc) == nil {
		switch tc.Type {
		case "auto":
			cr.ToolChoice = &CanonicalToolChoice{Mode: "auto"}
		case "any":
			cr.ToolChoice = &CanonicalToolChoice{Mode: "required"}
		case "tool":
			cr.ToolChoice = &CanonicalToolChoice{Mode: "tool", Name: tc.Name}
		}
	}
	if len(raw.System) > 0 {
		if sys := extractContentText(raw.System); sys != "" {
			cr.Messages = append(cr.Messages, Message{Role: "system", Content: sys})
		}
	}
	for _, m := range raw.Messages {
		cr.appendAnthropicContent(m.Role, m.Content)
	}
	return cr, nil
}

// appendAnthropicContent 解析 anthropic 消息 content：文本块并入消息文本；tool_use 块变为
// assistant 消息上的工具调用；tool_result 块变为独立的 role=tool 消息（openai 语义）。
func (cr *CanonicalRequest) appendAnthropicContent(role string, content json.RawMessage) {
	if len(content) == 0 {
		return
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		cr.Messages = append(cr.Messages, Message{Role: role, Content: s})
		return
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		// tool_use
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
		// tool_result
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return
	}
	text := ""
	var calls []CanonicalToolCall
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text += b.Text
		case "tool_use":
			calls = append(calls, CanonicalToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Input)})
		case "tool_result":
			cr.Messages = append(cr.Messages, Message{Role: "tool",
				ToolCallID: b.ToolUseID, Content: extractContentText(b.Content)})
		}
	}
	if text == "" && len(calls) == 0 {
		return
	}
	cr.Messages = append(cr.Messages, Message{Role: role, Content: text, ToolCalls: calls})
}

// extractContentText 兼容 string / [{type,text}] 两种 content 形态。
func extractContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return ""
}

// InputPlainText 汇总所有非 system 消息文本，供护栏检测。
func (cr *CanonicalRequest) InputPlainText() string {
	var sb strings.Builder
	for _, m := range cr.Messages {
		if m.Content == "" {
			continue
		}
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ---- 上游请求构建 ----

// BuildUpstreamRequest 按上游协议生成 HTTP 请求（model 替换为上游真实模型名）。
func BuildUpstreamRequest(upstreamProto Protocol, cr *CanonicalRequest, upstreamModel, baseURL, apiKey string) (*http.Request, error) {
	switch upstreamProto {
	case ProtoOpenAIChat, ProtoOpenAIResponses:
		return buildOpenAIRequest(upstreamProto, cr, upstreamModel, baseURL, apiKey)
	case ProtoAnthropic:
		return buildAnthropicRequest(cr, upstreamModel, baseURL, apiKey)
	}
	return nil, fmt.Errorf("unsupported upstream protocol %q", upstreamProto)
}

func joinURL(base, path string) string {
	return strings.TrimSuffix(base, "/") + path
}

func buildOpenAIRequest(p Protocol, cr *CanonicalRequest, model, baseURL, apiKey string) (*http.Request, error) {
	payload := map[string]any{"model": model, "stream": cr.Stream}
	if cr.MaxTokens > 0 {
		if p == ProtoOpenAIResponses {
			payload["max_output_tokens"] = cr.MaxTokens
		} else {
			payload["max_tokens"] = cr.MaxTokens
		}
	}
	if cr.Temperature != nil {
		payload["temperature"] = *cr.Temperature
	}
	if cr.TopP != nil {
		payload["top_p"] = *cr.TopP
	}
	var endpoint string
	if p == ProtoOpenAIResponses {
		var inputs []map[string]any
		for _, m := range cr.Messages {
			if m.Role == "system" {
				payload["instructions"] = m.Content
				continue
			}
			// assistant 历史工具调用 → function_call 项；工具结果 → function_call_output 项
			for _, tc := range m.ToolCalls {
				inputs = append(inputs, map[string]any{"type": "function_call",
					"call_id": tc.ID, "name": tc.Name, "arguments": tc.Arguments})
			}
			if m.Role == "tool" {
				inputs = append(inputs, map[string]any{"type": "function_call_output",
					"call_id": m.ToolCallID, "output": m.Content})
				continue
			}
			if len(m.ToolCalls) > 0 && m.Content == "" {
				continue // 工具调用已单独成项
			}
			inputs = append(inputs, map[string]any{"role": m.Role,
				"content": []map[string]any{{"type": m.roleContentType(), "text": m.Content}}})
		}
		payload["input"] = inputs
		if len(cr.Tools) > 0 {
			tools := make([]map[string]any, 0, len(cr.Tools))
			for _, t := range cr.Tools {
				tools = append(tools, map[string]any{"type": "function", "name": t.Name,
					"description": t.Description, "parameters": json.RawMessage(t.Parameters)})
			}
			payload["tools"] = tools
		}
		if tc := toolChoiceOpenAIResponses(cr.ToolChoice); tc != nil {
			payload["tool_choice"] = tc
		}
		endpoint = "/responses"
	} else {
		msgs := make([]map[string]any, 0, len(cr.Messages))
		for _, m := range cr.Messages {
			msg := map[string]any{"role": m.Role, "content": m.Content}
			if m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
			if m.Name != "" {
				msg["name"] = m.Name
			}
			if len(m.ToolCalls) > 0 {
				tcs := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					tcs = append(tcs, map[string]any{"id": tc.ID, "type": "function",
						"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
				}
				msg["tool_calls"] = tcs
			}
			msgs = append(msgs, msg)
		}
		payload["messages"] = msgs
		// 函数调用（openai_chat 上游，嵌套 function 形态）
		if len(cr.Tools) > 0 {
			tools := make([]map[string]any, 0, len(cr.Tools))
			for _, t := range cr.Tools {
				tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
					"name": t.Name, "description": t.Description, "parameters": json.RawMessage(t.Parameters)}})
			}
			payload["tools"] = tools
		}
		if tc := toolChoiceOpenAIChat(cr.ToolChoice); tc != nil {
			payload["tool_choice"] = tc
		}
		endpoint = "/chat/completions"
	}
	if cr.Stream {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	b, _ := json.Marshal(payload)
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1" // 兼容只填到域名的 base_url
	}
	req, err := http.NewRequest(http.MethodPost, base+endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}

func (m Message) roleContentType() string {
	if m.Role == "assistant" {
		return "output_text"
	}
	return "input_text"
}

// toolChoiceOpenAIChat 规范 → openai_chat tool_choice。
func toolChoiceOpenAIChat(tc *CanonicalToolChoice) any {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case "auto", "none", "required":
		return tc.Mode
	case "tool":
		return map[string]any{"type": "function", "function": map[string]any{"name": tc.Name}}
	}
	return nil
}

// toolChoiceOpenAIResponses 规范 → responses tool_choice（扁平结构）。
func toolChoiceOpenAIResponses(tc *CanonicalToolChoice) any {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case "auto", "none", "required":
		return tc.Mode
	case "tool":
		return map[string]any{"type": "function", "name": tc.Name}
	}
	return nil
}

// toolChoiceAnthropic 规范 → anthropic tool_choice（none 在 anthropic 无对应，省略）。
func toolChoiceAnthropic(tc *CanonicalToolChoice) any {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case "auto":
		return map[string]any{"type": "auto"}
	case "required":
		return map[string]any{"type": "any"}
	case "tool":
		return map[string]any{"type": "tool", "name": tc.Name}
	}
	return nil
}

func buildAnthropicRequest(cr *CanonicalRequest, model, baseURL, apiKey string) (*http.Request, error) {
	var system strings.Builder
	msgs := make([]map[string]any, 0, len(cr.Messages))
	for _, m := range cr.Messages {
		if m.Role == "system" {
			system.WriteString(m.Content)
			continue
		}
		// role=tool（openai 语义的工具结果）→ user 消息的 tool_result 块
		if m.Role == "tool" {
			msgs = append(msgs, map[string]any{"role": "user",
				"content": []map[string]any{{"type": "tool_result", "tool_use_id": m.ToolCallID, "content": m.Content}}})
			continue
		}
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user" // 其余角色归一为 user
		}
		// assistant 历史工具调用 → tool_use 块（anthropic input 必须是 JSON 对象）
		var content []map[string]any
		if m.Content != "" {
			content = append(content, map[string]any{"type": "text", "text": m.Content})
		}
		for _, tc := range m.ToolCalls {
			content = append(content, map[string]any{"type": "tool_use", "id": tc.ID,
				"name": tc.Name, "input": json.RawMessage(jsonOrObject(tc.Arguments))})
		}
		if len(content) == 0 {
			continue
		}
		msgs = append(msgs, map[string]any{"role": role, "content": content})
	}
	payload := map[string]any{"model": model, "messages": msgs, "stream": cr.Stream,
		"max_tokens": maxInt(cr.MaxTokens, 4096)}
	if len(cr.Tools) > 0 {
		tools := make([]map[string]any, 0, len(cr.Tools))
		for _, t := range cr.Tools {
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description,
				"input_schema": json.RawMessage(jsonOrObject(string(t.Parameters)))})
		}
		payload["tools"] = tools
	}
	if tc := toolChoiceAnthropic(cr.ToolChoice); tc != nil {
		payload["tool_choice"] = tc
	}
	if system.Len() > 0 {
		payload["system"] = system.String()
	}
	if cr.Temperature != nil {
		payload["temperature"] = *cr.Temperature
	}
	if cr.TopP != nil {
		payload["top_p"] = *cr.TopP
	}
	b, _ := json.Marshal(payload)
	path := "/v1/messages"
	if strings.HasSuffix(strings.TrimRight(baseURL, "/"), "/v1") {
		path = "/messages" // base_url 已含 /v1 时避免重复
	}
	req, err := http.NewRequest(http.MethodPost, joinURL(baseURL, path), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return req, nil
}

// jsonOrObject 把 JSON 字符串归一为合法 JSON 文本（空/非法时回退 "{}"，满足 anthropic
// input 必须为 JSON 对象的约束）。
func jsonOrObject(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "{}"
	}
	var v any
	if json.Unmarshal([]byte(t), &v) != nil {
		return "{}"
	}
	return t
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- 非流式响应解析 ----

func ParseCompletion(p Protocol, body []byte) (*CanonicalResponse, error) {
	switch p {
	case ProtoOpenAIChat:
		var r struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := &CanonicalResponse{ID: r.ID, Model: r.Model, FinishReason: "stop"}
		if len(r.Choices) > 0 {
			out.Content = r.Choices[0].Message.Content
			out.Reasoning = r.Choices[0].Message.Reasoning
			for _, tc := range r.Choices[0].Message.ToolCalls {
				out.ToolCalls = append(out.ToolCalls, CanonicalToolCall{ID: tc.ID,
					Name: tc.Function.Name, Arguments: tc.Function.Arguments})
			}
			if r.Choices[0].FinishReason != "" {
				out.FinishReason = r.Choices[0].FinishReason
			}
		}
		if r.Usage != nil {
			out.Usage = Usage{Prompt: r.Usage.PromptTokens, Completion: r.Usage.CompletionTokens}
		}
		return out, nil
	case ProtoAnthropic:
		var r struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
				// tool_use 块（input 为 JSON 对象）
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
			Usage      *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := &CanonicalResponse{ID: r.ID, Model: r.Model, FinishReason: mapStop(r.StopReason)}
		for _, c := range r.Content {
			switch c.Type {
			case "text":
				out.Content += c.Text
			case "tool_use":
				out.ToolCalls = append(out.ToolCalls, CanonicalToolCall{ID: c.ID,
					Name: c.Name, Arguments: string(c.Input)})
			}
		}
		if r.Usage != nil {
			out.Usage = Usage{Prompt: r.Usage.InputTokens, Completion: r.Usage.OutputTokens}
		}
		return out, nil
	case ProtoOpenAIResponses:
		var r struct {
			ID     string `json:"id"`
			Model  string `json:"model"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				// function_call 项
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"output"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := &CanonicalResponse{ID: r.ID, Model: r.Model, FinishReason: "stop"}
		for _, item := range r.Output {
			switch item.Type {
			case "message":
				for _, c := range item.Content {
					out.Content += c.Text
				}
			case "function_call":
				out.ToolCalls = append(out.ToolCalls, CanonicalToolCall{ID: item.CallID,
					Name: item.Name, Arguments: item.Arguments})
			}
		}
		if r.Usage != nil {
			out.Usage = Usage{Prompt: r.Usage.InputTokens, Completion: r.Usage.OutputTokens}
		}
		if r.Status == "incomplete" {
			out.FinishReason = "length"
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported protocol %q", p)
}

func mapStop(s string) string {
	switch s {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "":
		return "stop"
	}
	return "stop"
}

// ---- 错误响应（按客户端协议风格）----

func ErrorJSON(clientProto Protocol, httpStatus int, message, errType, code string) []byte {
	switch clientProto {
	case ProtoAnthropic:
		b, _ := json.Marshal(map[string]any{"type": "error",
			"error": map[string]any{"type": errType, "message": message}})
		return b
	default: // openai_chat / openai_responses
		b, _ := json.Marshal(map[string]any{"error": map[string]any{"message": message, "type": errType, "code": code}})
		return b
	}
}
