// loginlimit.go /auth/login 的 per-IP 固定窗口限速（SEC-08/M-03）：
// 无限速时锁定阈值既可被并发绕过（读改写竞态），也让任意 IP 免费慢速字典；
// 同时限制匿名请求触发 bcrypt 的成本（CPU 放大攻击面）。
package admin

import (
	"sync"
	"time"
)

const (
	loginIPWindowMax = 10          // 每窗口最多尝试次数
	loginIPWindowDur = time.Minute // 窗口长度
)

type loginAttemptWindow struct {
	count   int
	resetAt time.Time
}

// loginLimiter 线程安全的固定窗口计数。
type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*loginAttemptWindow
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{m: map[string]*loginAttemptWindow{}} }

// allow 记录一次尝试并返回是否放行。超过 mapSoftCap 时惰性清扫过期窗口（无后台协程）。
func (l *loginLimiter) allow(ip string, now time.Time) bool {
	const mapSoftCap = 8192
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) > mapSoftCap {
		for k, w := range l.m {
			if now.After(w.resetAt) {
				delete(l.m, k)
			}
		}
	}
	w := l.m[ip]
	if w == nil || now.After(w.resetAt) {
		l.m[ip] = &loginAttemptWindow{count: 1, resetAt: now.Add(loginIPWindowDur)}
		return true
	}
	if w.count >= loginIPWindowMax {
		return false
	}
	w.count++
	return true
}

// retryAfter 当前窗口剩余秒数（给 429 提示文案用）。
func (l *loginLimiter) retryAfter(ip string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w := l.m[ip]; w != nil && now.Before(w.resetAt) {
		return int(w.resetAt.Sub(now).Seconds()) + 1
	}
	return int(loginIPWindowDur.Seconds())
}
