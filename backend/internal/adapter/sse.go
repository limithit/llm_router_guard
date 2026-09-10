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
	TextDelta      string
	ReasoningDelta string          // 推理模型思维链增量（GLM/DeepSeek 风格 delta.reasoning_content）
	ToolCallDeltas []ToolCallDelta // 工具调用增量（协议无关；ID/Name 首片携带，ArgsDelta 跨片拼接）
	FinishReason   string          // 上游最终 finish_reason（stop/length/tool_calls/...，非 final 块为空）
	Usage          *Usage
	Finish         bool
	Err            string
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
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					ToolCalls []struct {
						Index    *int   `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
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
			out.ReasoningDelta = r.Choices[0].Delta.Reasoning
			for _, tc := range r.Choices[0].Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				out.ToolCallDeltas = append(out.ToolCallDeltas, ToolCallDelta{ToolIndex: idx,
					ID: tc.ID, Name: tc.Function.Name, ArgsDelta: tc.Function.Arguments})
			}
			if r.Choices[0].FinishReason != nil {
				out.Finish = *r.Choices[0].FinishReason != ""
				out.FinishReason = *r.Choices[0].FinishReason
			}
		}
		if r.Usage != nil {
			out.Usage = &Usage{Prompt: r.Usage.PromptTokens, Completion: r.Usage.CompletionTokens}
		}
		return out
	case ProtoAnthropic:
		var r struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"` // input_json_delta 的参数分片
			} `json:"delta"`
			ContentBlock *struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
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
		case "content_block_start":
			if r.ContentBlock != nil && r.ContentBlock.Type == "tool_use" {
				out.ToolCallDeltas = append(out.ToolCallDeltas, ToolCallDelta{ToolIndex: r.Index,
					ID: r.ContentBlock.ID, Name: r.ContentBlock.Name})
			}
		case "content_block_delta":
			switch r.Delta.Type {
			case "input_json_delta":
				out.ToolCallDeltas = append(out.ToolCallDeltas, ToolCallDelta{ToolIndex: r.Index,
					ArgsDelta: r.Delta.PartialJSON})
			default:
				out.TextDelta = r.Delta.Text
			}
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
			Type        string `json:"type"`
			Delta       string `json:"delta"`
			OutputIndex int    `json:"output_index"`
			Item        *struct {
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"item"`
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
		case "response.output_item.added":
			if r.Item != nil && r.Item.Type == "function_call" {
				out.ToolCallDeltas = append(out.ToolCallDeltas, ToolCallDelta{ToolIndex: r.OutputIndex,
					ID: r.Item.CallID, Name: r.Item.Name})
			}
		case "response.function_call_arguments.delta":
			out.ToolCallDeltas = append(out.ToolCallDeltas, ToolCallDelta{ToolIndex: r.OutputIndex,
				ArgsDelta: r.Delta})
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

	// ---- 工具调用跨协议转写状态 ----
	// anthropic：文本块固定 index 0；工具块从 1 起顺序分配；blocks 须 start→delta→stop 配对。
	anthropicTextOpen bool
	blockByTool       map[int]int // 上游 tool index → anthropic block index
	nextBlock         int
	openToolBlock     int // 当前未 stop 的工具块 index（-1=无）
	// responses：output 项计数与 completed 事件回填材料
	respText   strings.Builder
	fcItems    []map[string]any
	fcByID     map[int]*fcState // 上游 tool index → 项状态
	nextOutIdx int
}

type fcState struct {
	itemID    string
	callID    string
	name      string
	outputIdx int
	args      strings.Builder
	started   bool
}

func NewSSEWriter(clientProto Protocol, w http.ResponseWriter, respID, model string) *SSEWriter {
	fl, _ := w.(http.Flusher)
	return &SSEWriter{proto: clientProto, w: w, fl: fl, id: respID, model: model,
		openToolBlock: -1, blockByTool: map[int]int{}, fcByID: map[int]*fcState{}}
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
		s.anthropicTextOpen = true
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
		s.respText.WriteString(text)
		return s.raw("event: response.output_text.delta\ndata: " + fmt.Sprintf(
			`{"type":"response.output_text.delta","item_id":"msg_%s","output_index":0,"content_index":0,"delta":%s}`,
			s.id, string(tj)) + "\n\n")
	default:
		return s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{"content":%s},"finish_reason":null}]}`,
			s.id, s.model, string(tj)) + "\n\n")
	}
}

// Reasoning 输出推理模型思维链增量（delta.reasoning_content / anthropic thinking_delta）。
// OpenAI Responses 客户端协议暂不下发（无标准字段，静默跳过）。
func (s *SSEWriter) Reasoning(text string) error {
	if text == "" {
		return nil
	}
	tj, _ := json.Marshal(text)
	switch s.proto {
	case ProtoAnthropic:
		return s.raw("event: content_block_delta\ndata: " + fmt.Sprintf(
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":%s}}`, string(tj)) + "\n\n")
	case ProtoOpenAIResponses:
		return nil
	default:
		return s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{"reasoning_content":%s},"finish_reason":null}]}`,
			s.id, s.model, string(tj)) + "\n\n")
	}
}

// ToolCallDelta 输出工具调用增量，按客户端协议转写：
// openai_chat 重建 delta.tool_calls 分片；anthropic 转写为 content_block_start(tool_use) +
// input_json_delta（自动管理 block index 与文本块的开闭）；responses 转写为
// output_item.added + function_call_arguments.delta。
func (s *SSEWriter) ToolCallDelta(d ToolCallDelta) error {
	switch s.proto {
	case ProtoAnthropic:
		return s.toolCallDeltaAnthropic(d)
	case ProtoOpenAIResponses:
		return s.toolCallDeltaResponses(d)
	default:
		frag := map[string]any{"index": d.ToolIndex, "type": "function",
			"function": map[string]any{"arguments": d.ArgsDelta}}
		if d.ID != "" {
			frag["id"] = d.ID
		}
		if d.Name != "" {
			frag["function"] = map[string]any{"name": d.Name, "arguments": d.ArgsDelta}
		}
		b, _ := json.Marshal(frag)
		return s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{"tool_calls":[%s]},"finish_reason":null}]}`,
			s.id, s.model, string(b)) + "\n\n")
	}
}

func (s *SSEWriter) toolCallDeltaAnthropic(d ToolCallDelta) error {
	bi, ok := s.blockByTool[d.ToolIndex]
	if !ok {
		// 新工具块：先闭合文本块（若未闭合），再顺序分配 block index
		if s.anthropicTextOpen {
			if err := s.raw("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"); err != nil {
				return err
			}
			s.anthropicTextOpen = false
		}
		if s.openToolBlock >= 0 {
			if err := s.raw("event: content_block_stop\ndata: " + fmt.Sprintf(
				`{"type":"content_block_stop","index":%d}`, s.openToolBlock) + "\n\n"); err != nil {
				return err
			}
		}
		s.nextBlock++
		bi = s.nextBlock
		s.blockByTool[d.ToolIndex] = bi
		callID := d.ID
		if callID == "" {
			callID = fmt.Sprintf("toolu_%s_%d", s.id, bi)
		}
		name := d.Name
		if name == "" {
			name = "function"
		}
		s.openToolBlock = bi
		return s.raw("event: content_block_start\ndata: " + fmt.Sprintf(
			`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"%s","name":"%s","input":{}}}`,
			bi, callID, name) + "\n\n")
	}
	if d.ArgsDelta == "" {
		return nil
	}
	tj, _ := json.Marshal(d.ArgsDelta)
	return s.raw("event: content_block_delta\ndata: " + fmt.Sprintf(
		`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`,
		bi, string(tj)) + "\n\n")
}

func (s *SSEWriter) toolCallDeltaResponses(d ToolCallDelta) error {
	fc, ok := s.fcByID[d.ToolIndex]
	if !ok {
		s.nextOutIdx++
		itemID := fmt.Sprintf("fc_%s_%d", s.id, s.nextOutIdx)
		callID := d.ID
		if callID == "" {
			callID = itemID
		}
		fc = &fcState{itemID: itemID, callID: callID, name: d.Name,
			outputIdx: s.nextOutIdx, started: true}
		s.fcByID[d.ToolIndex] = fc
		s.fcItems = append(s.fcItems, nil) // 占位，completed 时按序回填
		ib, _ := json.Marshal(map[string]any{"type": "function_call", "id": itemID,
			"call_id": callID, "name": d.Name, "arguments": ""})
		if err := s.raw("event: response.output_item.added\ndata: " + fmt.Sprintf(
			`{"type":"response.output_item.added","output_index":%d,"item":%s}`,
			fc.outputIdx, string(ib)) + "\n\n"); err != nil {
			return err
		}
	}
	if d.Name != "" {
		fc.name = d.Name
	}
	if d.ArgsDelta == "" {
		return nil
	}
	fc.args.WriteString(d.ArgsDelta)
	tj, _ := json.Marshal(d.ArgsDelta)
	return s.raw("event: response.function_call_arguments.delta\ndata: " + fmt.Sprintf(
		`{"type":"response.function_call_arguments.delta","item_id":"%s","output_index":%d,"delta":%s}`,
		fc.itemID, fc.outputIdx, string(tj)) + "\n\n")
}

// Finalize 正常收尾（含 usage）。finish 为上游真实 finish_reason（stop/length/tool_calls/...），
// 空串按 stop 处理；anthropic 映射 stop→end_turn、length→max_tokens、tool_calls→tool_use，
// 并闭合所有已打开的 content block。
func (s *SSEWriter) Finalize(usage Usage, finish string) error {
	if finish == "" {
		finish = "stop"
	}
	switch s.proto {
	case ProtoAnthropic:
		// 闭合未关闭的块：文本块（无工具时）或最后一个工具块
		if s.anthropicTextOpen {
			if err := s.raw("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"); err != nil {
				return err
			}
			s.anthropicTextOpen = false
		}
		if s.openToolBlock >= 0 {
			if err := s.raw("event: content_block_stop\ndata: " + fmt.Sprintf(
				`{"type":"content_block_stop","index":%d}`, s.openToolBlock) + "\n\n"); err != nil {
				return err
			}
			s.openToolBlock = -1
		}
		stopReason := finish
		switch finish {
		case "stop":
			stopReason = "end_turn"
		case "length":
			stopReason = "max_tokens"
		case "tool_calls":
			stopReason = "tool_use"
		}
		return s.raw("event: message_delta\ndata: " + fmt.Sprintf(
			`{"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"output_tokens":%d}}`,
			stopReason, usage.Completion) + "\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	case ProtoOpenAIResponses:
		output := []any{}
		if s.respText.Len() > 0 || len(s.fcItems) == 0 {
			output = append(output, map[string]any{"type": "message", "id": "msg_" + s.id,
				"role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": s.respText.String(), "annotations": []any{}}}})
		}
		for _, fc := range s.fcByID {
			args := fc.args.String()
			if args == "" {
				args = "{}"
			}
			output = append(output, map[string]any{"type": "function_call", "id": fc.itemID,
				"call_id": fc.callID, "name": fc.name, "arguments": args, "status": "completed"})
		}
		tj, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{
			"id": s.id, "object": "response", "status": "completed", "model": s.model,
			"output": output,
			"usage":  map[string]any{"input_tokens": usage.Prompt, "output_tokens": usage.Completion}}})
		return s.raw("event: response.completed\ndata: " + string(tj) + "\n\n")
	default:
		err := s.raw("data: " + fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":0,"model":"%s","choices":[{"index":0,"delta":{},"finish_reason":"%s"}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
			s.id, s.model, finish, usage.Prompt, usage.Completion, usage.Prompt+usage.Completion) + "\n\n")
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
		content := []any{}
		if resp.Reasoning != "" {
			content = append(content, map[string]any{"type": "thinking", "thinking": resp.Reasoning})
		}
		if resp.Content != "" || len(resp.ToolCalls) == 0 {
			content = append(content, map[string]any{"type": "text", "text": json.RawMessage(tj)})
		}
		for i, tc := range resp.ToolCalls {
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("toolu_%s_%d", resp.ID, i+1)
			}
			content = append(content, map[string]any{"type": "tool_use", "id": id,
				"name": tc.Name, "input": json.RawMessage(jsonOrObject(tc.Arguments))})
		}
		stopReason := mapStopAnthropic(resp.FinishReason)
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "type": "message", "role": "assistant", "model": resp.Model,
			"content":     content,
			"stop_reason": stopReason,
			"usage":       map[string]any{"input_tokens": resp.Usage.Prompt, "output_tokens": resp.Usage.Completion}})
		return b
	case ProtoOpenAIResponses:
		output := []any{}
		if resp.Content != "" || len(resp.ToolCalls) == 0 {
			output = append(output, map[string]any{"type": "message", "id": "msg_" + resp.ID, "role": "assistant",
				"status":  "completed",
				"content": []any{map[string]any{"type": "output_text", "text": resp.Content, "annotations": []any{}}}})
		}
		for i, tc := range resp.ToolCalls {
			callID := tc.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%s_%d", resp.ID, i+1)
			}
			output = append(output, map[string]any{"type": "function_call", "id": "fc_" + callID,
				"call_id": callID, "name": tc.Name,
				"arguments": orEmptyObject(tc.Arguments), "status": "completed"})
		}
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "object": "response", "status": "completed", "model": resp.Model,
			"output": output,
			"usage": map[string]any{"input_tokens": resp.Usage.Prompt,
				"output_tokens": resp.Usage.Completion, "total_tokens": resp.Usage.Prompt + resp.Usage.Completion}})
		return b
	default: // openai_chat
		msg := map[string]any{"role": "assistant", "content": resp.Content}
		if resp.Reasoning != "" {
			msg["reasoning_content"] = resp.Reasoning
		}
		if len(resp.ToolCalls) > 0 {
			tcs := make([]map[string]any, 0, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				id := tc.ID
				if id == "" {
					id = fmt.Sprintf("call_%s_%d", resp.ID, i+1)
				}
				tcs = append(tcs, map[string]any{"id": id, "type": "function",
					"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
			}
			msg["tool_calls"] = tcs
		}
		b, _ := json.Marshal(map[string]any{
			"id": resp.ID, "object": "chat.completion", "created": 0, "model": resp.Model,
			"choices": []any{map[string]any{"index": 0,
				"message":       msg,
				"finish_reason": resp.FinishReason}},
			"usage": map[string]any{"prompt_tokens": resp.Usage.Prompt,
				"completion_tokens": resp.Usage.Completion,
				"total_tokens":      resp.Usage.Prompt + resp.Usage.Completion}})
		return b
	}
}

// mapStopAnthropic 规范 finish_reason → anthropic stop_reason。
func mapStopAnthropic(finish string) string {
	switch finish {
	case "stop", "":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	}
	return "end_turn"
}

// orEmptyObject 保证工具参数为合法 JSON 文本（responses/anthropic 客户端按对象解析）。
func orEmptyObject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
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
