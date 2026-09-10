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

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CanonicalRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
}

type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
}

type CanonicalResponse struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	Content      string `json:"content"`
	Reasoning    string `json:"reasoning"` // 推理模型思维链（GLM/DeepSeek reasoning_content）
	FinishReason string `json:"finish_reason"`
	Usage        Usage  `json:"usage"`
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
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw.Messages, &msgs); err != nil {
		return nil, fmt.Errorf("invalid messages: %w", err)
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
	for _, m := range msgs {
		cr.Messages = append(cr.Messages, Message{Role: m.Role, Content: extractContentText(m.Content)})
	}
	return cr, nil
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
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
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
	}
	if err := json.Unmarshal(raw.Input, &items); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	for _, it := range items {
		if it.Role == "" {
			continue
		}
		cr.Messages = append(cr.Messages, Message{Role: it.Role, Content: extractContentText(it.Content)})
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
		Stream      bool     `json:"stream"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature *float64 `json:"temperature"`
		TopP        *float64 `json:"top_p"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if raw.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	cr := &CanonicalRequest{Model: raw.Model, Stream: raw.Stream,
		MaxTokens: raw.MaxTokens, Temperature: raw.Temperature, TopP: raw.TopP}
	if len(raw.System) > 0 {
		if sys := extractContentText(raw.System); sys != "" {
			cr.Messages = append(cr.Messages, Message{Role: "system", Content: sys})
		}
	}
	for _, m := range raw.Messages {
		cr.Messages = append(cr.Messages, Message{Role: m.Role, Content: extractContentText(m.Content)})
	}
	return cr, nil
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
			inputs = append(inputs, map[string]any{"role": m.Role,
				"content": []map[string]any{{"type": m.roleContentType(), "text": m.Content}}})
		}
		payload["input"] = inputs
		endpoint = "/responses"
	} else {
		msgs := make([]map[string]any, 0, len(cr.Messages))
		for _, m := range cr.Messages {
			msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
		}
		payload["messages"] = msgs
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

func buildAnthropicRequest(cr *CanonicalRequest, model, baseURL, apiKey string) (*http.Request, error) {
	var system strings.Builder
	msgs := make([]map[string]any, 0, len(cr.Messages))
	for _, m := range cr.Messages {
		if m.Role == "system" {
			system.WriteString(m.Content)
			continue
		}
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user" // tool 等角色归一为 user
		}
		msgs = append(msgs, map[string]any{"role": role,
			"content": []map[string]any{{"type": "text", "text": m.Content}}})
	}
	payload := map[string]any{"model": model, "messages": msgs, "stream": cr.Stream,
		"max_tokens": maxInt(cr.MaxTokens, 4096)}
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
			if c.Type == "text" {
				out.Content += c.Text
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
			if item.Type == "message" {
				for _, c := range item.Content {
					out.Content += c.Text
				}
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
