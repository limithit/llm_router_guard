// sse.go 流式（SSE）支持：上游任意协议的增量解析 + 客户端任意协议的增量输出。
package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UpstreamChunk 上游 SSE data 行解析结果。
type UpstreamChunk struct {
	TextDelta string
	Usage     *Usage
	Finish    bool
	Err       string
}

// ParseUpstreamData 解析上游 "data:" 后的负载。
func ParseUpstreamData(p Protocol, data string) UpstreamChunk {
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return UpstreamChunk{Finish: data == "[DONE]"}
	}
	switch p {
	case ProtoOpenAIChat:
		var r struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &r) != nil {
			return UpstreamChunk{}
		}
		out := UpstreamChunk{}
		if r.Error != nil {
			out.Err = r.Error.Message
		}
		if len(r.Choices) > 0 {
			out.TextDelta = r.Choices[0].Delta.Content
			if r.Choices[0].FinishReason != nil {
				out.Finish = *r.Choices[0].FinishReason != ""
			}
		}
		if r.Usage != nil {
			out.Usage = &Usage{Prompt: r.Usage.PromptTokens, Completion: r.Usage.CompletionTokens}
		}
		return out
	case ProtoAnthropic:
		var r struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message *struct {
				Usage *struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &r) != nil {
			return UpstreamChunk{}
		}
		out := UpstreamChunk{}
		switch r.Type {
		case "content_block_delta":
			out.TextDelta = r.Delta.Text
		case "message_delta":
			if r.Usage != nil {
				out.Usage = &Usage{Completion: r.Usage.OutputTokens}
			}
		case "message_start":
			if r.Message != nil && r.Message.Usage != nil {
				out.Usage = &Usage{Prompt: r.Message.Usage.InputTokens}
			}
		case "message_stop":
			out.Finish = true
		case "error":
			if r.Error != nil {
				out.Err = r.Error.Message
			}
		}
		return out
	case ProtoOpenAIResponses:
		var r struct {
			Type   string `json:"type"`
			Delta  string `json:"delta"`
			Response *struct {
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(data), &r) != nil {
			return UpstreamChunk{}
		}
		out := UpstreamChunk{}
		switch r.Type {
		case "response.output_text.delta":
			out.TextDelta = r.Delta
		case "response.completed", "response.incomplete", "response.failed":
			out.Finish = true
			if r.Response != nil && r.Response.Usage != nil {
				out.Usage = &Usage{Prompt: r.Response.Usage.InputTokens, Completion: r.Response.Usage.OutputTokens}
			}
		}
		return out
	}
	return UpstreamChunk{}
}

// ---- 客户端 SSE 输出 ----

// SSEWriter 把规范增量事件转写为客户端协议 SSE。
type SSEWriter struct {
	proto  Protocol
	w      io.Writer
	fl     http.Flusher
	id     string
	model  string
	opened bool
	failed bool
}

func NewSSEWriter(clientProto Protocol, w http.ResponseWriter, respID, model string) *SSEWriter {
	fl, _ := w.(http.Flusher)
	return &SSEWriter{proto: clientProto, w: w, fl: fl, id: respID, model: model}
}

func (s *SSEWriter) flush() {
	if s.fl != nil {
		s.fl.Flush()
	}
}

func (s *SSEWriter) raw(line string) error {
	_, err := io.WriteString(s.w, line)
	if err == nil {
		s.flush()
	}
	return err
}

// Open 输出协议要求的起始事件。
func (s *SSEWriter) Open() error {
	s.opened = true
	switch s.proto {
	case ProtoAnthropic:
		return s.raw("event: message_start\ndata: " + fmt.Sprintf(
			`{"type":"message_start","message":{"id":"%s","type":"message","role":"assistant","model":"%s","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`,
			s.id, s.model) + "\n\n" +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	case ProtoOpenAIResponses:
		return s.raw("event: response.created\ndata: " + fmt.Sprintf(
			`{"type":"response.created","response":{"id":"%s","object":"response","status":"in_progress","model":"%s","output":[]}}`,
			s.id, s.model) + "\n\n")
	default:
		return s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			s.id, s.model) + "\n\n")
	}
}

// Delta 输出一段增量文本。
func (s *SSEWriter) Delta(text string) error {
	if text == "" {
		return nil
	}
	tj, _ := json.Marshal(text)
	switch s.proto {
	case ProtoAnthropic:
		return s.raw("event: content_block_delta\ndata: " + fmt.Sprintf(
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, string(tj)) + "\n\n")
	case ProtoOpenAIResponses:
		return s.raw("event: response.output_text.delta\ndata: " + fmt.Sprintf(
			`{"type":"response.output_text.delta","item_id":"msg_%s","output_index":0,"content_index":0,"delta":%s}`,
			s.id, string(tj)) + "\n\n")
	default:
		return s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{"content":%s},"finish_reason":null}]}`,
			s.id, s.model, string(tj)) + "\n\n")
	}
}

// Finalize 正常收尾（含 usage）。
func (s *SSEWriter) Finalize(usage Usage) error {
	switch s.proto {
	case ProtoAnthropic:
		return s.raw("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
			"event: message_delta\ndata: " + fmt.Sprintf(
				`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":%d}}`,
				usage.Completion) + "\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	case ProtoOpenAIResponses:
		tj, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{
			"id": s.id, "object": "response", "status": "completed", "model": s.model,
			"output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{}}},
			"usage":  map[string]any{"input_tokens": usage.Prompt, "output_tokens": usage.Completion}}})
		return s.raw("event: response.completed\ndata: " + string(tj) + "\n\n")
	default:
		err := s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
			s.id, s.model, usage.Prompt, usage.Completion, usage.Prompt+usage.Completion) + "\n\n")
		if err != nil {
			return err
		}
		return s.raw("data: [DONE]\n\n")
	}
}

// Fail 流中途出错时输出协议错误事件并终止。
func (s *SSEWriter) Fail(message string) error {
	s.failed = true
	tj, _ := json.Marshal(message)
	switch s.proto {
	case ProtoAnthropic:
		return s.raw("event: error\ndata: " + fmt.Sprintf(
			`{"type":"error","error":{"type":"api_error","message":%s}}`, string(tj)) + "\n\n")
	case ProtoOpenAIResponses:
		return s.raw("event: response.failed\ndata: " + fmt.Sprintf(
			`{"type":"response.failed","response":{"id":"%s","status":"failed","error":{"message":%s}}}`,
			s.id, string(tj)) + "\n\n")
	default:
		return s.raw("data: " + fmt.Sprintf(`{"error":{"message":%s,"type":"server_error"}}`, string(tj)) +
			"\n\n" + "data: [DONE]\n\n")
	}
}

// BuildCompletionJSON 非流式响应按客户端协议序列化。
func BuildCompletionJSON(clientProto Protocol, resp *CanonicalResponse) []byte {
	switch clientProto {
	case ProtoAnthropic:
		tj, _ := json.Marshal(resp.Content)
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "type": "message", "role": "assistant", "model": resp.Model,
			"content": []any{map[string]any{"type": "text", "text": json.RawMessage(tj)}},
			"stop_reason": resp.FinishReason,
			"usage": map[string]any{"input_tokens": resp.Usage.Prompt, "output_tokens": resp.Usage.Completion}})
		return b
	case ProtoOpenAIResponses:
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "object": "response", "status": "completed", "model": resp.Model,
			"output": []any{map[string]any{"type": "message", "id": "msg_" + resp.ID, "role": "assistant",
				"status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": resp.Content, "annotations": []any{}}}}},
			"usage": map[string]any{"input_tokens": resp.Usage.Prompt,
				"output_tokens": resp.Usage.Completion, "total_tokens": resp.Usage.Prompt + resp.Usage.Completion}})
		return b
	default: // openai_chat
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "object": "chat.completion", "created": 0, "model": resp.Model,
			"choices": []any{map[string]any{"index": 0,
				"message": map[string]any{"role": "assistant", "content": resp.Content},
				"finish_reason": resp.FinishReason}},
			"usage": map[string]any{"prompt_tokens": resp.Usage.Prompt,
				"completion_tokens": resp.Usage.Completion,
				"total_tokens":      resp.Usage.Prompt + resp.Usage.Completion}})
		return b
	}
}

// ExtractError 从上游非 200 响应中提取错误信息。
func ExtractError(p Protocol, body []byte) string {
	var o struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &o) == nil && o.Error.Message != "" {
		return o.Error.Message
	}
	var a struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &a) == nil && a.Error.Message != "" {
		return a.Error.Message
	}
	if len(body) > 200 {
		body = body[:200]
	}
	return string(body)
}
