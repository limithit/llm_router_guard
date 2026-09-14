// auditws.go 实时审计 WebSocket 端点（P2 #5）。
//
// GET /api/admin/v1/audit/ws?token=<JWT>
//   - 浏览器 WebSocket 无法自定义 Authorization 头，token 走查询串；
//   - 鉴权失败在升级前拒绝（101 之前返回 401，不产生半开连接）；
//   - 升级后每条调用审计落库即推送一帧 JSON（model.CallLog 序列化，与
//     /audit/calls 列表字段一致，前端可复用类型）；
//   - 30s ping 保活；慢消费者丢弃计数（hub），堆积可达阈值时主动 1013 关闭；
//   - 客户端 close / 断连自动摘除订阅。

package admin

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/auth"
)

// wsPingInterval 保活周期（覆盖常见 LB 60s 空闲超时）。
const wsPingInterval = 30 * time.Second

// wsSubscriberSlowLimit 单订阅者积压丢弃达到该值即判定消费端失能，主动断开（1013 Try Again Later）。
const wsSubscriberSlowLimit = 512

// auditWS 实时调用审计流。
// sameOriginHost M-14：Origin(scheme://host[:port]) 的主机部分必须与请求 Host 相同。
func sameOriginHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == host
}

func (s *Server) auditWS(c *gin.Context) {
	// 1) 查询串 JWT 鉴权（浏览器 WS 无法带 Authorization 头）
	claims, err := auth.Parse(s.secret, c.Query("token"))
	if err != nil || claims == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "Token 无效或已过期，请重新登录"})
		return
	}

	// 1b) M-14：CSWSH 防线——浏览器发起的 WS 握手必带 Origin（不可被脚本伪造），
	// 要求其与请求 Host 同源；无 Origin 头视为非浏览器客户端放行（运维工具兼容）。
	if origin := c.Request.Header.Get("Origin"); origin != "" && !sameOriginHost(origin, c.Request.Host) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40302, "message": "cross-origin websocket request rejected"})
		return
	}

	// 2) HTTP → WebSocket 升级（RFC6455 §4.2，校验必要头）
	h := c.Request.Header
	if h.Get("Upgrade") != "websocket" || h.Get("Connection") == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "not a websocket handshake"})
		return
	}
	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "connection hijack unsupported"})
		return
	}
	key := h.Get("Sec-WebSocket-Key")
	if key == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "missing Sec-WebSocket-Key"})
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	// 3) 先订阅广播中心，再发 101。
	// 顺序很关键：101 一旦落 TCP，客户端即认为连接就绪并可触发审计写入；
	// 若订阅后置，此窗口内的事件会静默丢失（push 型功能最忌讳的竞态）。
	msgs, cancel := s.al.Broadcaster().Subscribe()
	defer cancel()

	// 4) 101 响应（升级握手）——直接写 Hijack 返回的 rw 并 Flush 落 TCP
	_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n")
	if err == nil {
		err = rw.Flush()
	}
	if err != nil {
		return
	}

	done := make(chan struct{})
	go func() { // 读泵：仅处理控制帧（close → 断开；ping → pong），数据帧读过即弃
		defer close(done)
		for {
			op, payload, err := wsReadClientFrame(rw.Reader)
			if err != nil {
				return
			}
			switch op {
			case wsOpClose:
				_ = wsWriteCloseFrame(rw, 1000)
				return
			case wsOpPing:
				if err := wsWriteFrame(rw, wsOpPong, payload); err != nil {
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	dropBase := s.al.Broadcaster().Dropped()
loop:
	for {
		select {
		case <-done: // 客户端断开
			break loop
		case msg, ok := <-msgs:
			if !ok {
				break loop
			}
			if err := wsWriteFrame(rw, wsOpText, msg); err != nil {
				break loop
			}
			if d := s.al.Broadcaster().Dropped() - dropBase; d >= wsSubscriberSlowLimit {
				_ = wsWriteCloseFrame(rw, 1013) // 消费端失能：主动断开，前端重连即恢复
				break loop
			}
		case <-ticker.C:
			if err := wsWriteFrame(rw, wsOpPing, nil); err != nil {
				break loop
			}
		}
	}
}
