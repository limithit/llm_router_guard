#!/usr/bin/env python3
"""Mock OpenAI-compatible upstream for gateway E2E tests.

Endpoints:
  GET  /v1/models            -> model list (strict test-connection check)
  POST /v1/chat/completions  -> non-stream JSON / SSE stream, both with usage
  POST /__shutdown           -> exits the process (test cleanup)

Behavior driven by user message content:
  contains __TRIGGER_OUTPUT__ -> reply contains SECRETWORD (output-guard test)
  otherwise                   -> reply is echo(<content>)

Stream tuning (env, default = 2 chunks/50ms like the original):
  MOCK_STREAM_CHUNKS  -> number of content chunks (interleaved slices, concat=reply)
  MOCK_STREAM_DELAY    -> seconds to sleep between chunks (simulate slow upstream)
"""
import json
import os
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ.get("MOCK_PORT", "18081"))
MODE = os.environ.get("MOCK_MODE", "good")  # good=正常回包; bad=chat 一律 500（熔断验证用）


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def _json(self, obj, status=200):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/v1/models":
            self._json({"object": "list", "data": [
                {"id": "mock-small", "object": "model"},
                {"id": "mock-large", "object": "model"},
            ]})
        else:
            self._json({"error": {"message": "not found"}}, 404)

    def do_POST(self):
        if self.path == "/__shutdown":
            self._json({"ok": True})
            threading.Thread(target=self.server.shutdown, daemon=True).start()
            return
        if self.path != "/v1/chat/completions":
            self._json({"error": {"message": "not found"}}, 404)
            return
        length = int(self.headers.get("Content-Length") or 0)
        body = json.loads(self.rfile.read(length) or b"{}")
        if MODE == "bad":
            print("CHAT bad-500", flush=True)
            self._json({"error": {"message": "mock upstream failure",
                                  "type": "mock_error"}}, 500)
            return
        model = body.get("model", "mock-small")
        content = ""
        for m in reversed(body.get("messages", [])):
            if m.get("role") == "user":
                c = m.get("content")
                content = c if isinstance(c, str) else json.dumps(c)
                break
        if "__TRIGGER_OUTPUT__" in content:
            reply = "The model says: SECRETWORD leaked into output."
        else:
            reply = "echo(%s)" % content
        if body.get("stream"):
            self._stream(reply, model)
        else:
            self._json({
                "id": "cmpl-mock", "object": "chat.completion",
                "created": int(time.time()), "model": model,
                "choices": [{"index": 0,
                             "message": {"role": "assistant", "content": reply},
                             "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 42, "completion_tokens": 17, "total_tokens": 59},
            })

    def _stream(self, reply, model):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()
        chunks = int(os.environ.get("MOCK_STREAM_CHUNKS", "2"))
        delay = float(os.environ.get("MOCK_STREAM_DELAY", "0.05"))
        tid = "cmpl-mock-%s" % os.getpid()
        # 连续等分：恰好 chunks 段、拼接恒等于 reply（chunks=2 时等同原始前后对半）
        n = len(reply)
        pieces = [reply[i * n // chunks:(i + 1) * n // chunks] for i in range(chunks)]
        for part in pieces:
            chunk = {"id": tid, "object": "chat.completion.chunk",
                     "created": int(time.time()), "model": model,
                     "choices": [{"index": 0, "delta": {"content": part},
                                  "finish_reason": None}]}
            self.wfile.write(b"data: " + json.dumps(chunk).encode() + b"\n\n")
            self.wfile.flush()
            time.sleep(delay)
        final = {"id": tid, "object": "chat.completion.chunk",
                 "created": int(time.time()), "model": model, "choices": [],
                 "usage": {"prompt_tokens": 11, "completion_tokens": 5,
                           "total_tokens": 16}}
        self.wfile.write(b"data: " + json.dumps(final).encode() + b"\n\n")
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
