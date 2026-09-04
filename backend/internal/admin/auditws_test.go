package admin

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/audit"
	"llmrouter/internal/auth"
	"llmrouter/internal/model"
)

// newWSTestServer 起一个带 /api/admin/v1/audit/ws 的最小 admin Server，
// 返回服务实例（供测试注入审计事件）与可拨号的 base URL / token。
func newWSTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{secret: "test-secret", al: audit.NewLogger(nil, func() time.Duration { return 0 })}
	r := gin.New()
	r.GET("/api/admin/v1/audit/ws", s.auditWS)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	tok, err := auth.Issue("test-secret", 1, "tester")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return s, ts.URL, tok
}

// wsDial 原生 TCP 完成 RFC6455 握手，返回连接。
func wsDial(t *testing.T, base, token string) net.Conn {
	t.Helper()
	host := strings.TrimPrefix(base, "http://")
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	wsKey := base64.StdEncoding.EncodeToString(key)
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	req := "GET /api/admin/v1/audit/ws?token=" + token + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + wsKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write upgrade: %v", err)
	}
	br := bufio.NewReader(conn)
	status := ""
	haveAccept := false
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read upgrade response: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "HTTP/") {
			status = line
		}
		if strings.HasPrefix(line, "Sec-WebSocket-Accept:") {
			haveAccept = true
		}
	}
	if !strings.Contains(status, "101") {
		conn.Close()
		t.Fatalf("handshake status = %q, want 101", status)
	}
	if !haveAccept {
		conn.Close()
		t.Fatal("missing Sec-WebSocket-Accept")
	}
	return conn
}

// wsClientMaskedFrame 发送一个客户端帧（浏览器帧必须掩码）。
func wsClientMaskedFrame(t *testing.T, conn net.Conn, opcode byte, payload []byte) {
	t.Helper()
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	frame := []byte{0x80 | opcode, 0x80 | byte(len(payload))}
	frame = append(frame, mask...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("write client frame: %v", err)
	}
}

func TestAuditWSRejectsBadToken(t *testing.T) {
	_, base, _ := newWSTestServer(t)
	resp, err := http.Get(base + "/api/admin/v1/audit/ws?token=bad-token")
	if err != nil {
		t.Fatalf("bad token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("bad token status = %d, want 401 (before upgrade)", resp.StatusCode)
	}
}

func TestAuditWSHandshakePushAndClose(t *testing.T) {
	s, base, token := newWSTestServer(t)
	conn := wsDial(t, base, token)
	defer conn.Close()

	// 订阅建立后写入一条审计 → 应收到一帧 JSON 文本
	s.al.Write(&model.CallLog{RequestID: "req-ws-1", ModelAlias: "gpt-4o", Status: "ok"})

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	op, payload, err := wsReadClientFrame(br)
	if err != nil {
		t.Fatalf("read push frame: %v", err)
	}
	if op != wsOpText {
		t.Fatalf("opcode = %#x, want text(0x1)", op)
	}
	var row map[string]any
	if err := json.Unmarshal(payload, &row); err != nil {
		t.Fatalf("frame not json: %v (%q)", err, payload)
	}
	if row["request_id"] != "req-ws-1" || row["model_alias"] != "gpt-4o" {
		t.Errorf("pushed row = %v", row)
	}

	// 客户端主动 close → 服务端回 close 帧并结束读循环
	wsClientMaskedFrame(t, conn, wsOpClose, []byte{0x03, 0xE8})
	op, _, err = wsReadClientFrame(br)
	if err != nil {
		t.Fatalf("read close reply: %v", err)
	}
	if op != wsOpClose {
		t.Errorf("close reply opcode = %#x, want 0x8", op)
	}
}

func TestAuditWSClientPingGetsPong(t *testing.T) {
	_, base, token := newWSTestServer(t)
	conn := wsDial(t, base, token)
	defer conn.Close()

	wsClientMaskedFrame(t, conn, wsOpPing, []byte("hb"))
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	op, payload, err := wsReadClientFrame(br)
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if op != wsOpPong || string(payload) != "hb" {
		t.Errorf("got opcode=%#x payload=%q, want pong 'hb'", op, payload)
	}
}
