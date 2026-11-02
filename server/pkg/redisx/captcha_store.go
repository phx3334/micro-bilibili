package redisx

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 编译期接口校验：确保 RedisCaptchaStore 满足 base64Captcha.Store 接口的全部方法。
// 当前 base64Captcha v1.3.8 的 Store 接口包含 Set / Get / Verify 三个方法。
// 若未来 base64Captcha 升级新增方法（例如 Clear），编译器会立即报错提醒补齐。
var _ base64Captcha.Store = (*RedisCaptchaStore)(nil)

// captchaExpire 验证码在 Redis 中的 TTL，默认 5 分钟。
// 该值与 base64Captcha.DefaultMemStore 的 Expiration=10min 不同，
// 这里选择更短的过期时间以减小重放窗口。
const captchaExpire = 5 * time.Minute

// RedisCaptchaStore 是基于 Redis 的 base64Captcha Store 实现。
//
// 设计要点：
//   - 键命名空间：使用 BuildKey("captcha", id) 拼出完整键，自动复用 RedisClient.KeyPrefix
//   - 值格式：直接存储验证码答案字符串（{answer}），过期时间由 Redis TTL 托管
//   - 原子消费：Verify 通过 Get(clear=true) + strings.EqualFold 实现，
//     由于 Get/Verify 在同一次 RTT 内完成（Redis 单线程），可视为原子消费
//
// 并发安全说明：
//   - 底层 *redis.Client 由 go-redis v9 自身保证并发安全
//   - 本结构体无内部状态，零锁开销
//   - 多 API 实例共享同一 Redis，所有实例可读/写同一份 captcha 数据
//
// 失败语义：
//   - Set 失败：返回 error，由调用方决定是否重试或回退到内存 store
//   - Get 返回空字符串：表示验证码不存在 / 已过期 / Get 出错（统一视为失效）
//   - Verify 返回 false：答案不匹配、id 不存在、answer 为空、Set 后被并发 Verify 抢走
type RedisCaptchaStore struct {
	client *RedisClient
}

// NewRedisCaptchaStore 构造一个 RedisCaptchaStore 实例。
// 参数 client 必须为同包下的 RedisClient，会复用其 KeyPrefix 命名空间。
func NewRedisCaptchaStore(client *RedisClient) *RedisCaptchaStore {
	return &RedisCaptchaStore{client: client}
}

// Set 将验证码答案写入 Redis。
//
// 存储策略：
//   - 键：{KeyPrefix}:captcha:{id}
//   - 值：answer 字符串
//   - TTL：captchaExpire（5 分钟）
//
// 返回值：
//   - 成功返回 nil
//   - 写入失败返回 Redis 错误（连接断开、AOF rewrite 期间等）
//
// 注意：base64Captcha 框架在调用 Set 时会传入 keyAnswer 格式
// （id=实际 captchaID，value=答案字符串），本实现直接透传 id 作为键后缀。
func (s *RedisCaptchaStore) Set(id string, value string) error {
	if id == "" {
		return errors.New("captcha id is empty")
	}
	key := s.client.BuildKey("captcha", id)
	ctx, cancel := context.WithTimeout(context.Background(), captchaExpire)
	defer cancel()
	// 使用 SET key value EX <seconds> 一步完成写入与 TTL 设置，
	// 避免 SET + EXPIRE 两步操作之间崩溃导致 key 永不过期的边界情况。
	return s.client.Client.Set(ctx, key, value, captchaExpire).Err()
}

// Get 读取指定 id 对应的验证码答案。
//
// 参数 clear：
//   - true：使用 GETDEL 原子获取并删除（用于"校验即失效"语义）
//   - false：使用 GET 保留键（用于"先查后验"等场景，本项目目前未使用）
//
// 返回值：
//   - 成功：返回答案字符串
//   - 失败：统一返回空字符串（key 不存在 / 已过期 / Redis 异常）
//
// base64Captcha.Store.Get 的接口约定是返回 string，不返回 error，
// 因此本方法对所有错误情况（含 redis.Nil）一律返回空字符串，由调用方
// 依据"答案是否为空"判定是否成功。
func (s *RedisCaptchaStore) Get(id string, clear bool) string {
	if id == "" {
		return ""
	}
	key := s.client.BuildKey("captcha", id)
	ctx, cancel := context.WithTimeout(context.Background(), captchaExpire)
	defer cancel()

	if clear {
		// GETDEL：Redis 6.2+ 提供的原子"获取并删除"指令。
		// 原子性保证：并发场景下两个请求同时 Verify 同一 id 时，
		// 只有一个能拿到非空值，另一个拿到空串，从而天然防重放。
		// 与"先 GET 再 DEL"两步操作相比，避免了 GET 成功但 DEL 之前
		// 进程崩溃导致 key 永远存在的内存泄漏风险。
		val, err := s.client.Client.GetDel(ctx, key).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				// 修复：区分"键不存在"和"Redis 不可用"
				// redis.Nil 是正常的业务场景（验证码已过期或被消费）
				// 其他错误（连接超时、Redis 宕机等）应记录警告日志
				// 注意：受 base64Captcha.Store 接口限制（只返回 string），无法向上层传递错误，
				// 但至少应记录日志便于排查 Redis 不可用时验证码全部失效的问题
				zap.L().Warn("Redis GetDel 异常，验证码校验将失败",
					zap.String("id", id), zap.Error(err))
			}
			return ""
		}
		return val
	}

	// 仅读取，不删除。用于前端"先回显一下校验码"等场景。
	val, err := s.client.Client.Get(ctx, key).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			zap.L().Warn("Redis Get 异常，验证码校验将失败",
				zap.String("id", id), zap.Error(err))
		}
		return ""
	}
	return val
}

// Verify 校验指定 id 的答案是否正确。
//
// 校验流程：
//  1. 调用 Get(id, true) 原子获取并删除（clear=true 触发 GETDEL）
//  2. 若 id 或 answer 为空，直接返回 false（与 base64Captcha 默认实现保持一致）
//  3. 使用 strings.EqualFold 进行大小写不敏感比较
//
// 并发安全：
//
//	同一 id 多次并发 Verify 时，由于 GETDEL 的原子性，只有第一个 Verify 能拿到原答案，
//	后续 Verify 拿到空串后必然返回 false。这正是"一次性使用"的语义。
//
// 返回值：
//   - true：id 存在 + answer 大小写不敏感匹配成功
//   - false：id 不存在、已过期、已验证过、answer 不匹配
func (s *RedisCaptchaStore) Verify(id, answer string, clear bool) bool {
	if id == "" || answer == "" {
		return false
	}
	value := s.Get(id, clear)
	if value == "" {
		return false
	}
	// 大小写不敏感比较：与 base64Captcha 默认 memoryStore.Verify 保持一致。
	// 验证码通常由数字+字母组成，用户输入大小写不应对结果有影响。
	return strings.EqualFold(value, answer)
}
