// Package lock 提供基于 Redis 的分布式锁
//
// 用途：
//   - 在多 worker 部署场景下，保证"视频热度 ZSet 重建"等重活同时只有一个实例执行
//   - 单 API + 单 worker 的当前部署下也保留该能力，便于后续横向扩展
//
// 实现要点（教科书级 Redis 分布式锁最小实现）：
//  1. 加锁：SET key token NX EX ttl —— 原子写入 + 设置过期时间（防止死锁）
//  2. 解锁：Lua 脚本 "GET == token 才 DEL" —— 防止误删别人的锁
//  3. 续期：Lua 脚本 "GET == token 才 PEXPIRE" —— 防止长任务执行期间锁过期
//
// 安全保证：
//   - NX 保证互斥：同一时刻只有一个 Acquire 能成功
//   - token 唯一性：调用方传入 token（一般用 hostname + 时间戳），防止
//     进程 A 加锁后阻塞、锁过期、进程 B 拿到锁、进程 A 醒来误删 B 的锁
//   - TTL 兜底：即便持锁进程崩溃，锁也会在 ttl 之后自动释放
//   - 续期原子性：通过 Lua 脚本在 Redis 服务端单线程执行，避免 check-then-act
//
// 已知限制：
//   - 不支持"等待锁释放"语义（没有 TryLock 带重试的版本），
//     调用方在 ErrLockHeld 时应主动放弃本次任务，等下一轮 cron 周期
//   - 时钟漂移不影响正确性（服务端 Redis 负责 TTL），但会影响"近似公平"
package lock

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrLockHeld 表示锁已被其他持有者持有。
//
// 触发场景：
//   - 另一个 worker 已经先一步加锁
//   - 上一次 cron 周期执行超时，锁还未过期
//
// 调用方应按业务决定行为（cron 任务里一般直接 return nil 跳过本轮）。
var ErrLockHeld = errors.New("redis lock already held")

// Lock 表示一把 Redis 分布式锁。
//
// 字段说明：
//   - client：go-redis 客户端，复用现有连接池
//   - key：锁对应的 Redis 键（如 "cron:rebuild_zset"）
//   - token：锁持有者标识（用于 Release / Refresh 时的身份校验）
//
// 生命周期：
//   - Acquire 成功 → 获得 *Lock
//   - defer l.Release(ctx) 在临界区结束时释放
//   - 长任务可在临界区内定时调用 l.Refresh(ctx, ttl) 续期
type Lock struct {
	client *redis.Client
	key    string
	token  string
}

// Acquire 尝试以 SET key token NX EX ttl 原子方式获取锁。
//
// 参数：
//   - ctx：调用方上下文；ctx 取消时 SETNX 调用立即返回
//   - client：go-redis 客户端
//   - key：锁对应的 Redis 键
//   - token：锁持有者标识，必须全局唯一（建议用 hostname + 时间戳）
//   - ttl：锁的过期时间；ttl 必须 > 0，保证即便持锁进程崩溃锁也能被回收
//
// 返回：
//   - 成功：*Lock + nil，调用方需在临界区结束时 defer Release
//   - 失败且锁被持有：nil + ErrLockHeld
//   - 失败且网络错误：nil + error
func Acquire(ctx context.Context, client *redis.Client, key, token string, ttl time.Duration) (*Lock, error) {
	ok, err := client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLockHeld
	}
	return &Lock{client: client, key: key, token: token}, nil
}

// Refresh 续期（仅当仍持有时才续）。
//
// 用 Lua 脚本保证 check-and-set 原子：
//   - GET 拿当前值，与 token 比对
//   - 一致则 PEXPIRE 重置 TTL
//   - 不一致（已被其他持有者接管）则返回 0，调用方拿到 ErrLockHeld
//
// 使用场景：cron 任务执行时间 > ttl 时（如 RebuildZSet 在大数据量下
// 跑几分钟），必须在任务内部定时调用 Refresh，否则任务还没跑完锁
// 就过期了，第二个 worker 会同时进入临界区，破坏互斥语义。
//
// 注意：ttl 应当与 Acquire 时的 ttl 一致，或略长于"任务单次执行时间"，
// 否则会出现"频繁续期但任务依然超时"的窘境。
func (l *Lock) Refresh(ctx context.Context, ttl time.Duration) error {
	// Lua 脚本：原子地"读-比-续"
	// KEYS[1] = 锁键
	// ARGV[1] = token（持锁者标识）
	// ARGV[2] = 新 TTL（毫秒）
	script := redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    return 0
end
`)
	res, err := script.Run(ctx, l.client, []string{l.key}, l.token, ttl.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if res == 0 {
		// 锁已被其他持有者接管（很可能是我们这边 ttl 过期了）
		return ErrLockHeld
	}
	return nil
}

// Release 释放锁（用 Lua 脚本保证 check-and-del 原子）。
//
// 不做"持锁者身份校验"的话会出现这个经典 bug：
//  1. 进程 A 加锁，TTL=10s
//  2. 进程 A 业务执行 15s（超过了 TTL）
//  3. 10s 时锁自动过期
//  4. 进程 B 拿到锁
//  5. 15s 时进程 A 终于执行完，调用 DEL —— 误删进程 B 的锁
//
// 用 Lua 在 Redis 服务端单线程执行"GET == token 才 DEL"即可避免。
// 释放失败（网络错误）不会回滚业务，但下一轮 cron 周期会重试，幂等
// 性由业务层保证。
func (l *Lock) Release(ctx context.Context) error {
	script := redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)
	_, err := script.Run(ctx, l.client, []string{l.key}, l.token).Result()
	return err
}
