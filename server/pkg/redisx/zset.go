package redisx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ZIncrBy 为有序集合中指定成员增加分数。
// 若成员不存在则创建，score 可为负数。
func (r *RedisClient) ZIncrBy(ctx context.Context, key string, members string, score float64) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZIncrBy(ctx, key, score, members).Err()
}

// ZAdd 向有序集合添加一个或多个成员。支持批量添加，成员的 Score 决定其在 ZSet 中的排序位置。
func (r *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZAdd(ctx, key, members...).Err()
}

// ZRemRangeByRank 按排名范围删除成员（0-based，从低分到高分排序）。
// start=0, stop=-1 表示删除全部成员。
func (r *RedisClient) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRemRangeByRank(ctx, key, start, stop).Err()
}

// ZRevRangeWithScores 按分数从高到低返回指定排名范围的成员及分数（0-based）。
// start=0, stop=9 返回 Top 10。
func (r *RedisClient) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   key,
		Start: start,
		Stop:  stop,
		Rev:   true, // Rev=true 表示从高分到低分
	}).Result()
}

// ZRangeWithScores 按分数从低到高返回指定排名范围的成员及分数（0-based）。
func (r *RedisClient) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   key,
		Start: start,
		Stop:  stop,
		Rev:   false, // Rev=false 表示从低分到高分
	}).Result()
}

// Expire 为指定键设置过期时间（TTL）。
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.Expire(ctx, key, ttl).Err()
}

// ZCard 获取有序集合中的成员数量。
func (r *RedisClient) ZCard(ctx context.Context, key string) (int64, error) {
	if r.Client == nil {
		return 0, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZCard(ctx, key).Result()
}

// Exists 检查键是否存在。
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	if r.Client == nil {
		return false, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	n, err := r.Client.Exists(ctx, key).Result()
	return n > 0, err
}

// ZRevRange 按分数从高到低返回指定排名范围的成员（不含分数）。
func (r *RedisClient) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:   key,
		Start: start,
		Stop:  stop,
		Rev:   true,
	}).Result()
}

// ZUnionStore 计算多个有序集合的并集并存储到目标键。
// aggregate 支持 "SUM"、"MIN"、"MAX"，决定重复成员的分数聚合方式。
func (r *RedisClient) ZUnionStore(ctx context.Context, dst string, keys []string, aggregate string) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZUnionStore(ctx, dst, &redis.ZStore{
		Keys:      keys,
		Aggregate: aggregate,
	}).Err()
}

// ZRevRangeByScoreWithScores 按分数范围（从高到低）查询成员，返回成员及分数。
// opt 参数允许设置 Min/Max/Offset/Count 实现更灵活的范围查询。
func (r *RedisClient) ZRevRangeByScoreWithScores(ctx context.Context, key string, opt *redis.ZRangeBy) ([]redis.Z, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRevRangeByScoreWithScores(ctx, key, opt).Result()
}

// ZRem 按成员（视频 ID 字符串）从有序集合移除单个成员。
// 视频删除时调用，将其从热度 ZSet（全局 + 分区）中剔除。
func (r *RedisClient) ZRem(ctx context.Context, key string, member string) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRem(ctx, key, member).Err()
}

// ZRemRangeByScore 按分数范围删除成员。
// min/max 支持 "(" 前缀表示开区间，如 "(100" 表示 score>100。
func (r *RedisClient) ZRemRangeByScore(ctx context.Context, key, min, max string) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZRemRangeByScore(ctx, key, min, max).Err()
}

// ZScore 获取指定成员的当前分数。
func (r *RedisClient) ZScore(ctx context.Context, key, member string) (float64, error) {
	if r.Client == nil {
		return 0, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.ZScore(ctx, key, member).Result()
}

// ZSetCursorPaginate 实现基于 ZSet 的游标分页（高分优先），正确处理同分元素的边界情况。
//
// 通过 (cursorScore, cursorMember) 双字段游标精确定位分页位置：
//   - cursorScore == 0 且 cursorMember == "" 时表示第一页
//   - 后续页通过 ZREVRANK 定位游标成员的排名，从其下一个位置开始取数据
//   - 若游标成员已被删除（ZREVRANK 返回 redis.Nil），降级为纯分数范围查询
//
// 返回值:
//   - members: 当前页的成员列表（含分数）
//   - nextCursorScore: 下一页游标分数（0 表示无下一页）
//   - nextCursorMember: 下一页游标成员 ID（空串表示无下一页）
//
// 游标编码格式为 base64(json([score, id]))，与 MySQL 游标分页保持一致。
func (r *RedisClient) ZSetCursorPaginate(ctx context.Context, key string, cursorScore float64, cursorMember string, limit int64) ([]redis.Z, float64, string, error) {
	if r.Client == nil {
		return nil, 0, "", ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()

	if cursorScore == 0 && cursorMember == "" {
		members, err := r.Client.ZRevRangeWithScores(ctx, key, 0, limit).Result()
		if err != nil {
			return nil, 0, "", err
		}
		if int64(len(members)) <= limit {
			return members, 0, "", nil
		}
		last := members[limit-1]
		return members[:limit], last.Score, last.Member.(string), nil
	}

	rank, err := r.Client.ZRevRank(ctx, key, cursorMember).Result()
	if err != nil {
		members, fallbackErr := r.Client.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Max:    fmt.Sprintf("(%.0f", cursorScore),
			Min:    "-inf",
			Offset: 0,
			Count:  limit + 1,
		}).Result()
		if fallbackErr != nil {
			return nil, 0, "", fallbackErr
		}
		if int64(len(members)) <= limit {
			return members, 0, "", nil
		}
		last := members[limit-1]
		return members[:limit], last.Score, last.Member.(string), nil
	}

	members, err := r.Client.ZRevRangeWithScores(ctx, key, rank+1, rank+1+limit).Result()
	if err != nil {
		return nil, 0, "", err
	}
	if int64(len(members)) <= limit {
		return members, 0, "", nil
	}
	last := members[limit-1]
	return members[:limit], last.Score, last.Member.(string), nil
}

// ZSetBatchAdd 通过 Pipeline 批量向 ZSet 添加成员，显著减少网络往返次数。
// 适用于定时任务全量重建 ZSet 的场景。
func (r *RedisClient) ZSetBatchAdd(ctx context.Context, key string, members []redis.Z) error {
	if len(members) == 0 {
		return nil
	}
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	pipe := r.Client.Pipeline()
	for _, m := range members {
		pipe.ZAdd(ctx, key, m)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// IsHotVideo 判断视频是否在热门 ZSet 中（即是否为热门视频）
// 只有热门视频才需要互斥锁防击穿，冷门视频并发量低不需要
func (r *RedisClient) IsHotVideo(ctx context.Context, videoID uint) bool {
	zsetKey := r.BuildKey(PublishedVideoZSetKey, "global")
	member := strconv.FormatUint(uint64(videoID), 10)
	_, err := r.ZScore(ctx, zsetKey, member)
	return err == nil // ZScore 无错说明成员存在
}
