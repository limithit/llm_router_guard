package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"llmrouter/internal/model"
)

// TestValidProtocol 协议枚举校验：三个合法值 + 典型错误值（含大小写敏感——网关转发是精确匹配）。
func TestValidProtocol(t *testing.T) {
	for _, p := range []string{"openai_chat", "openai_responses", "anthropic"} {
		if !validProtocol(p) {
			t.Errorf("validProtocol(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"openai", "anthropic_msg", "OpenAI_Chat", "", "chat"} {
		if validProtocol(p) {
			t.Errorf("validProtocol(%q) = true, want false", p)
		}
	}
}

// TestCreateProvider_RejectsInvalidProtocol 第九轮实测发现：错误协议值此前一路存库，
// 直到转发时才 502（unsupported upstream protocol）。现在必须在创建时拒绝。
func TestCreateProvider_RejectsInvalidProtocol(t *testing.T) {
	s := newTestServer(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/providers", s.createProvider)

	w := httptest.NewRecorder()
	body := `{"name":"bad-proto","protocol":"openai","base_url":"http://127.0.0.1:9"}`
	req := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid protocol: got HTTP %d want 400 (body %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "protocol") {
		t.Fatalf("error message should mention protocol, got %s", w.Body.String())
	}
	// 确认未落库
	var count int64
	s.db.Model(&model.Provider{}).Where("name = ?", "bad-proto").Count(&count)
	if count != 0 {
		t.Fatal("invalid provider must not be persisted")
	}
}

// TestUpdateProvider_RejectsInvalidProtocol 更新同样受枚举约束。
func TestUpdateProvider_RejectsInvalidProtocol(t *testing.T) {
	s := newTestServer(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 更新成功路径依赖 s.mgr（recordOp），这里用 seed 直建记录 + 只测拒绝路径
	id := s.seedProvider(t, "p1", "openai_chat", "http://127.0.0.1:9", "k")
	r.PUT("/providers/:id", s.updateProvider)

	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"name":"p1","protocol":"gpt4","base_url":"http://127.0.0.1:9"}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/providers/%d", id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid protocol on update: got HTTP %d want 400 (body %s)", w.Code, w.Body.String())
	}
	var p model.Provider
	if err := s.db.First(&p, id).Error; err != nil || p.Protocol != "openai_chat" {
		t.Fatalf("protocol must remain unchanged after rejected update: %v %q", err, p.Protocol)
	}
}
