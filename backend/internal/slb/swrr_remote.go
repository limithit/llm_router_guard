package slb

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"llmrouter/internal/runtime"
)

// swrrRemote — 全局 SWRR 游标（#16，多节点）。
//
// 把 SWRR 的一步推进放进 Redis Lua 原子执行：HGETALL 游标 → 按当前候选
// （健康过滤由调用方完成）推进 currentWeight → 选最大 → 选中者减 total →
// HSET 写回。多实例对同一别名的轮询序列互不重叠，严格按权重比例分流。
//
// 键：gw:swrr:v1:<alias>（HASH providerID → currentWeight）。
// 降级：任何 Redis 错误 → 置 redisDown + 后台探活，返回 ok=false 让 Pick
// 回落实例本地游标（调用方零等待，与熔断共享状态降级语义一致）。

// swrrKey 全局游标键。
func swrrKey(alias string) string { return fmt.Sprintf("gw:swrr:v1:%s", alias) }

// swrrScript SWRR 一步推进（KEYS[1]=游标 HASH；ARGV: 2N 个 "id","weight" 对）。
// 返回被选中 providerID。与内存 swrrPick 语义一致（并列取首个）。
var swrrScript = redis.NewScript(`
local cw = KEYS[1]
local n = #ARGV / 2
local best_id, best_cw, total = nil, nil, 0
for i = 1, n do
  local id  = ARGV[2 * i - 1]
  local w   = tonumber(ARGV[2 * i])
  local cur = tonumber(redis.call('HGET', cw, id) or '0') or 0
  cur = cur + w
  redis.call('HSET', cw, id, cur)
  total = total + w
  if best_id == nil or cur > best_cw then
    best_id, best_cw = id, cur
  end
end
if best_id ~= nil then
  redis.call('HSET', cw, best_id, best_cw - total)
end
return best_id
`)

// swrrRemote 执行一步全局游标推进。candidates 须已含健康过滤与 tried 过滤。
func (bl *Balancer) swrrRemote(alias string, candidates []runtime.ResolvedUpstream) (runtime.ResolvedUpstream, bool) {
	if len(candidates) == 0 {
		return runtime.ResolvedUpstream{}, false
	}
	var args []interface{}
	for _, u := range candidates {
		args = append(args, strconv.FormatUint(uint64(u.ProviderID), 10), u.Weight)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res, err := swrrScript.Run(ctx, bl.rdb, []string{swrrKey(alias)}, args...).Result()
	if err != nil {
		log.Printf("[slb] redis swrr advance failed (%v): degrading alias %q to local cursor", err, alias)
		bl.redisDown.set(true)
		go bl.watchRedis()
		return runtime.ResolvedUpstream{}, false
	}
	idStr, _ := res.(string)
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		// 理论不可达（脚本只返回 ARGV 里的 id 或 nil）
		return runtime.ResolvedUpstream{}, false
	}
	for _, u := range candidates {
		if uint64(u.ProviderID) == id64 {
			return u, true
		}
	}
	return runtime.ResolvedUpstream{}, false
}
