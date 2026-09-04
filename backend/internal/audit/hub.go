// hub.go 实时审计广播中心（P2 #5）：audit.Logger 写入的每条调用日志
// 同时推送给所有已连接的 WebSocket 管理端订阅者（实时调用流）。
// 设计：
//   - 每订阅者带 64 缓冲 channel，慢消费者不阻塞写入主链路；
//     缓冲满即丢弃该条并计数（监控语义优先于完整性，完整数据仍在 DB/CSV 导出）；
//   - 订阅者断开（context 取消）自动从扇出列表摘除；多读者并发安全。
package audit

import (
	"sync"
)

const hubBuffer = 64 // 单订阅者缓冲；满即丢弃（软实时流，不阻塞写库路径）

// subscriber 一个 WebSocket 订阅者。
type subscriber struct {
	ch chan []byte
}

// Hub 调用日志广播中心（线程安全）。
type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
	drop int64 // 因慢消费丢弃的总条数（可观测）
}

// NewHub 创建广播中心。
func NewHub() *Hub {
	return &Hub{subs: map[*subscriber]struct{}{}}
}

// Subscribe 注册订阅者，返回接收 channel 与取消函数。
// channel 关闭或 cancel 调用后订阅结束。
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	sub := &subscriber{ch: make(chan []byte, hubBuffer)}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub.ch, func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}
}

// PublishNonBlocking 向所有订阅者非阻塞广播一条 JSON 编码消息；
// 单订阅者缓冲满 → 丢弃并计数（绝不阻塞 audit 写入路径）。
func (h *Hub) PublishNonBlocking(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		select {
		case sub.ch <- msg:
		default:
			h.drop++
		}
	}
}

// Subscribers 当前订阅数（/status 可观测）。
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Dropped 累计丢弃条数。
func (h *Hub) Dropped() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.drop
}
