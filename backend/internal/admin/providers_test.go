package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/crypto"
	"llmrouter/internal/db"
	"llmrouter/internal/model"
)

// newTestServer 构造一个仅含 DB + 加密器的最小 admin Server（testProvider 只依赖这两者）。
func newTestServer(t *testing.T) *Server {
	t.Helper()
	gdb, err := db.Open("sqlite", filepath.Join(t.TempDir(), "providers_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	enc, err := crypto.NewCipher("test-master-key")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	if sqlDB, err := gdb.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() }) // 释放文件句柄，便于 TempDir 清理
	}
	return &Server{db: gdb, enc: enc}
}

func (s *Server) seedProvider(t *testing.T, name, proto, baseURL, apiKey string) uint {
	t.Helper()
	enc, err := s.enc.Encrypt(apiKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	p := model.Provider{Name: name, Protocol: proto, BaseURL: baseURL, APIKeyEnc: enc, Enabled: true}
	if err := s.db.Create(&p).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return p.ID
}

func callTest(t *testing.T, s *Server, id uint) (bool, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/providers/:id/test", s.testProvider)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/providers/"+fmt.Sprint(id)+"/test", nil)
	r.ServeHTTP(w, req)
	var resp struct {
		Data struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return resp.Data.OK, resp.Data.Message
}

// TestTestProvider 覆盖各类上游响应，确保错误端点不再误报「连接成功」。
func TestTestProvider(t *testing.T) {
	s := newTestServer(t)

	// 1) 错误端点：返回 404 —— 旧逻辑会误报成功，新逻辑必须失败。
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer srv404.Close()

	// 2) 正常模型列表：200 + data 数组 —— 唯一应判定成功的场景。
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`))
	}))
	defer srvOK.Close()

	// 3) 认证失败：401。
	srv401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv401.Close()

	// 4) 可达但非模型列表（200 + 异形 JSON）—— 应判失败。
	srvBadShape := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer srvBadShape.Close()

	// 5) 端口不可达：连接失败。
	srvDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srvDownURL := srvDown.URL
	srvDown.Close()

	cases := []struct {
		name    string
		proto   string
		baseURL string
		wantOK  bool
		wantSub string
	}{
		{"wrong-endpoint-404", "openai_chat", srv404.URL, false, "404"},
		{"valid-models-list", "openai_chat", srvOK.URL, true, "连接成功"},
		{"auth-fail-401", "anthropic", srv401.URL, false, "认证失败"},
		{"non-standard-200", "openai_chat", srvBadShape.URL, false, "非标准"},
		{"connection-refused", "openai_chat", srvDownURL, false, "连接失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := s.seedProvider(t, tc.name, tc.proto, tc.baseURL, "sk-fake-key")
			ok, msg := callTest(t, s, id)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v (message=%q)", ok, tc.wantOK, msg)
			}
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("message %q does not contain %q", msg, tc.wantSub)
			}
		})
	}
}
