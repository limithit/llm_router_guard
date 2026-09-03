// Package tokens 提供 token 计数：优先使用 tiktoken（cl100k_base BPE）精确分词，
// 编码器不可用（离线且无缓存）时回退到启发式估算。
// 配额/用量统计依赖本包估算上游未返回 usage 的场景（网关转发路径）。
package tokens

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// 默认编码器：cl100k_base（GPT-4 / GPT-3.5-turbo 系列）。
// 首次使用需从网络下载 BPE 编码文件并缓存；离线部署可设置
// TIKTOKEN_CACHE_DIR 指向预置缓存目录。
const defaultEncoding = "cl100k_base"

var (
	initOnce sync.Once
	enc      *tiktoken.Tiktoken
	encErr   error
)

// Init 预热编码器（建议在服务启动时异步调用），避免首个请求承担下载/解析延迟。
// 返回初始化错误；失败时 Count 自动降级为启发式估算。
func Init() error {
	initOnce.Do(func() {
		enc, encErr = tiktoken.GetEncoding(defaultEncoding)
	})
	return encErr
}

// Available 报告精确编码器是否可用。
func Available() bool {
	Init()
	return encErr == nil
}

// Count 返回文本的 token 数；空文本返回 0。
// 编码器不可用时回退到 Estimate。
func Count(text string) int {
	if text == "" {
		return 0
	}
	Init()
	if encErr == nil && enc != nil {
		return len(enc.EncodeOrdinary(text))
	}
	return Estimate(text)
}

// Estimate 启发式估算（离线回退）：CJK 字符约 2 字符 / token，
// 其余字符约 4 字符 / token，至少记 1。
func Estimate(text string) int {
	runes := []rune(text)
	cjk := 0
	for _, r := range runes {
		if isCJK(r) {
			cjk++
		}
	}
	other := len(runes) - cjk
	n := cjk/2 + other/4 + 1
	return n
}

// isCJK 粗判中日韩统一表意文字及常用符号区。
func isCJK(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x9FFF, // CJK 部首/康熙/表意文字
		r >= 0xF900 && r <= 0xFAFF, // CJK 兼容表意
		r >= 0x3000 && r <= 0x303F, // CJK 标点
		r >= 0xFF00 && r <= 0xFFEF, // 全角形式
		r >= 0x3040 && r <= 0x30FF, // 假名
		r >= 0xAC00 && r <= 0xD7AF: // 谚文
		return true
	}
	return false
}
