package audit

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"llmrouter/internal/model"
)

func TestHubFanoutAndUnsubscribe(t *testing.T) {
	h := NewHub()
	ch1, cancel1 := h.Subscribe()
	ch2, cancel2 := h.Subscribe()
	if h.Subscribers() != 2 {
		t.Fatalf("subscribers = %d, want 2", h.Subscribers())
	}

	h.PublishNonBlocking([]byte(`{"request_id":"r1"}`))
	for i, ch := range []<-chan []byte{ch1, ch2} {
		got := <-ch
		if string(got) != `{"request_id":"r1"}` {
			t.Errorf("sub%d got %q", i, got)
		}
	}
	if h.Dropped() != 0 {
		t.Errorf("dropped = %d, want 0", h.Dropped())
	}

	cancel1()
	cancel2()
	// 摘除后无订阅者：发布零开销且不丢
	h.PublishNonBlocking([]byte(`{"request_id":"r2"}`))
	if h.Subscribers() != 0 || h.Dropped() != 0 {
		t.Errorf("after cancel: subs=%d dropped=%d, want 0/0", h.Subscribers(), h.Dropped())
	}
}

func TestHubSlowConsumerDrops(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe()
	defer cancel()

	// 灌满订阅缓冲（64）后再多发若干条 → 计入丢弃
	for i := 0; i < 64+10; i++ {
		h.PublishNonBlocking([]byte("x"))
	}
	if h.Dropped() != 10 {
		t.Errorf("dropped = %d, want 10", h.Dropped())
	}
}

func TestLoggerBroadcastsToHub(t *testing.T) {
	al := NewLogger(nil, func() time.Duration { return 0 })
	ch, cancel := al.Broadcaster().Subscribe()
	defer cancel()

	al.Write(&model.CallLog{RequestID: "req-live-1", ModelAlias: "gpt-4o", Status: "ok"})

	select {
	case b := <-ch:
		var row map[string]any
		if err := json.Unmarshal(b, &row); err != nil {
			t.Fatalf("broadcast not json: %v", err)
		}
		if row["request_id"] != "req-live-1" || row["model_alias"] != "gpt-4o" {
			t.Errorf("broadcast fields = %v", row)
		}
	default:
		t.Fatal("no broadcast received for live write")
	}
}

func TestLoggerNoSubscriberNoCost(t *testing.T) {
	al := NewLogger(nil, func() time.Duration { return 0 })
	// 无订阅者：Write 不 panic、不广播（JSON 不编码）
	al.Write(&model.CallLog{RequestID: "req-x", Status: "ok"})
	if al.Broadcaster().Subscribers() != 0 {
		t.Errorf("unexpected subscribers")
	}
}

// 并发发布安全（-race）
func TestHubConcurrentPublish(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.PublishNonBlocking([]byte("m"))
			}
		}()
	}
	wg.Wait()
	// 至少收到部分消息即可（其余可能被丢弃——正是非阻塞语义）
	n := 0
	for n < 1 {
		select {
		case <-ch:
			n++
		default:
			if n == 0 {
				t.Fatal("expected at least one message delivered")
			}
		}
	}
}
