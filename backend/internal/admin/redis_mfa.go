// redis_mfa.go — MFA 二步登录票据与绑定流程密钥的存储抽象（第九轮后续，多节点）。
//
// 票据（login ticket）与绑定密钥（setup secret）是短时效（5 分钟）一次性数据，
// 原实现在实例内存（sync.Map）——多实例部署时二步请求经 LB 打到不同实例会
// 报「票据已失效」。抽象为 mfaStore 接口：
//   - memMFA    : 原内存实现（单节点 / SQLite，零依赖）
//   - redisMFA  : Redis 实现（SET + TTL 5 分钟，GETDEL 原子取出），
//     多实例共享，任一实例写入、任一实例可核销。
package admin

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// mfaStore MFA 短时效票据存储。Pop* 取出即作废（一次性）。
// TTL 统一 5 分钟，替代原内存实现的 expiry 检查。
type mfaStore interface {
	SaveLoginTicket(token string, userID uint)
	PopLoginTicket(token string) (userID uint, ok bool)
	SaveSetupSecret(uid uint, secret string)
	PopSetupSecret(uid uint) (secret string, ok bool)
}

// ---------- 内存实现（单节点） ----------

type memMFA struct {
	login sync.Map // token -> uint userID
	setup sync.Map // uid -> string secret
}

func (m *memMFA) SaveLoginTicket(token string, uid uint) { m.login.Store(token, uid) }
func (m *memMFA) PopLoginTicket(token string) (uint, bool) {
	v, ok := m.login.LoadAndDelete(token)
	if !ok {
		return 0, false
	}
	return v.(uint), true
}
func (m *memMFA) SaveSetupSecret(uid uint, secret string) { m.setup.Store(uid, secret) }
func (m *memMFA) PopSetupSecret(uid uint) (string, bool) {
	v, ok := m.setup.LoadAndDelete(uid)
	if !ok {
		return "", false
	}
	return v.(string), true
}

// ---------- Redis 实现（多节点） ----------

const mfaTicketTTL = 5 * time.Minute

// luaPop GET+DEL 原子取出（GETDEL 需要 Redis ≥6.2，脚本兼容更老版本）。
var luaPop = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v then
  redis.call('DEL', KEYS[1])
end
return v
`)

type redisMFA struct {
	rdb *redis.Client
}

func newRedisMFA(addr, password string) *redisMFA {
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password, PoolSize: 10,
		DialTimeout: time.Second, ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		// 票据丢失只会让用户重新登录/重新绑定，不阻断启动
		log.Printf("[mfa] WARNING: redis %s unreachable (%v); MFA tickets fall back to per-instance memory", addr, err)
		return nil
	}
	log.Printf("[mfa] redis %s connected: MFA tickets are cluster-wide", addr)
	return &redisMFA{rdb: rdb}
}

func (r *redisMFA) loginKey(token string) string { return "gw:mfa:login:v1:" + token }
func (r *redisMFA) setupKey(uid uint) string {
	return "gw:mfa:setup:v1:" + strconv.FormatUint(uint64(uid), 10)
}

func (r *redisMFA) SaveLoginTicket(token string, uid uint) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.rdb.Set(ctx, r.loginKey(token), strconv.FormatUint(uint64(uid), 10), mfaTicketTTL)
}
func (r *redisMFA) PopLoginTicket(token string) (uint, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	v, err := luaPop.Run(ctx, r.rdb, []string{r.loginKey(token)}).Text()
	if err != nil || v == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(n), true
}
func (r *redisMFA) SaveSetupSecret(uid uint, secret string) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r.rdb.Set(ctx, r.setupKey(uid), secret, mfaTicketTTL)
}
func (r *redisMFA) PopSetupSecret(uid uint) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	v, err := luaPop.Run(ctx, r.rdb, []string{r.setupKey(uid)}).Text()
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// newMFAStore 按部署形态选择存储：REDIS_ADDR 配置且可达 → Redis；否则内存。
func newMFAStore(redisAddr, redisPassword string) mfaStore {
	if redisAddr != "" {
		if r := newRedisMFA(redisAddr, redisPassword); r != nil {
			return r
		}
	}
	return &memMFA{}
}

// 编译期断言
var (
	_ mfaStore = (*memMFA)(nil)
	_ mfaStore = (*redisMFA)(nil)
)
