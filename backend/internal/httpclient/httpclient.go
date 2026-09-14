// Package httpclient 出站 HTTP 客户端统一工厂（M-01 加固）。
//
// 问题背景：Go 的 http.Client 在跨主机重定向时只剥 `Authorization` 头，
// 自定义的 `x-api-key`（anthropic 协议）会原样带到重定向目标——
// 恶意/被接管的供应商端点可用一次 302 把 API Key 转送给任意主机。
//
// 策略：LLM 供应商/健康检查端点本就不应依赖重定向，网关一律拒绝 3xx：
// 重定向错误以明确的 error 返回（进审计与故障转移判定），而非静默泄密。
// 如确有需要跟随重定向的出站调用，请显式自建客户端并注明理由。
package httpclient

import (
	"errors"
	"net/http"
	"time"
)

// ErrRedirectRejected 请求被服务端重定向——已按安全策略拒绝跟随。
var ErrRedirectRejected = errors.New("redirect refused (possible credential leak via x-api-key, see M-01)")

// New 返回带安全重定向策略的客户端。timeout<=0 表示不设总超时（流式场景）。
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirectRejected
		},
	}
}

// NewTransported 供网关使用：连接池参数定制 + 安全重定向策略。
func NewTransported(timeout time.Duration, transport *http.Transport) *http.Client {
	c := New(timeout)
	c.Transport = transport
	return c
}
