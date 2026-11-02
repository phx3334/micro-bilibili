package video

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/httpx"
	"fake_tiktok/pkg/util"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// VideoLogic 视频业务逻辑层
type VideoLogic struct {
	deps *LogicDeps

	// sfGroup 用于缓存击穿保护：同一个 videoID 的并发回源请求只执行一次。
	sfGroup singleflight.Group

	// zoneCaches 分区热门视频短期缓存 + 互斥锁，防缓存击穿
	// key=zone|cursor|limit，value=*zoneCache
	zoneCaches sync.Map
}

// zoneCache 单个分区（或全局）的热门视频缓存 + 互斥锁
type zoneCache struct {
	mu       sync.Mutex
	result   *hotVideosCache
	expireAt time.Time
}

// hotVideosCache 热门视频列表缓存结果
type hotVideosCache struct {
	list       []httpx.HomeVideoInfo
	total      int64
	nextCursor string
}

func NewVideoLogic(deps *LogicDeps) *VideoLogic {
	return &VideoLogic{deps: deps}
}

// DeleteVideo 作者删除自己投稿的视频。
func (v *VideoLogic) DeleteVideo(ctx context.Context, userID string, videoID uint) error {
	video, err := v.deps.VideoRepo.FindVideoByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("视频不存在")
		}
		return fmt.Errorf("查询失败: %w", err)
	}
	if video.AuthorID != userID {
		return errors.New("无权限删除")
	}

	// 3. 软删除
	if err := v.deps.VideoRepo.DeleteVideo(ctx, videoID); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}

	// 4. 尽力清理关联资源
	v.cleanupDeletedVideo(ctx, video)
	return nil
}

// cleanupDeletedVideo 删除视频后的关联资源清理（失败不致命）。
func (v *VideoLogic) cleanupDeletedVideo(ctx context.Context, video *model.Video) {
	logger := v.deps.Logger

	// 4.1 删除对象存储中的视频/封面（DB 中保存的就是对象 key）
	for _, key := range []string{video.PlayURL, video.CoverURL} {
		if key == "" {
			continue
		}
		if err := v.deps.Storage.Delete(ctx, key); err != nil && logger != nil {
			logger.Warn("delete video: remove storage object failed",
				zap.Uint("video_id", video.ID), zap.String("key", key), zap.Error(err))
		}
	}

	// 4.2 删除 Redis 视频缓存
	if v.deps.VideoCacheRepo != nil {
		if err := v.deps.VideoCacheRepo.DeleteVideoCache(ctx, video.ID); err != nil && logger != nil {
			logger.Warn("delete video: clear cache failed",
				zap.Uint("video_id", video.ID), zap.Error(err))
		}
	}

	// 4.3 从热度 ZSet 移除（全局 + 分区）
	if v.deps.RedisClient != nil {
		member := strconv.FormatUint(uint64(video.ID), 10)
		if err := v.deps.RedisClient.ZRem(ctx, v.deps.RedisClient.BuildPublishedZSetKey(""), member); err != nil && logger != nil {
			logger.Warn("delete video: remove from global zset failed",
				zap.Uint("video_id", video.ID), zap.Error(err))
		}
		if video.Zone != "" {
			if err := v.deps.RedisClient.ZRem(ctx, v.deps.RedisClient.BuildPublishedZSetKey(video.Zone), member); err != nil && logger != nil {
				logger.Warn("delete video: remove from zone zset failed",
					zap.Uint("video_id", video.ID), zap.String("zone", video.Zone), zap.Error(err))
			}
		}
	}
}

// mediaURL 把 DB/缓存中的对象 key 转换成可访问地址；已是绝对 URL 的旧值原样返回。
func (v *VideoLogic) mediaURL(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	u, err := v.deps.Storage.URL(ctx, key)
	if err != nil {
		if v.deps.Logger != nil {
			v.deps.Logger.Warn("storage url sign failed", zap.String("key", key), zap.Error(err))
		}
		return ""
	}
	return u
}

// signHomeList 在响应序列化阶段对封面/作者头像做 URL 转换（缓存与 DB 中只存 key）。
func (v *VideoLogic) signHomeList(ctx context.Context, list []httpx.HomeVideoInfo) []httpx.HomeVideoInfo {
	if list == nil {
		return nil
	}
	out := make([]httpx.HomeVideoInfo, 0, len(list))
	for _, item := range list {
		item.CoverURL = v.mediaURL(ctx, item.CoverURL)
		item.UpAvatar = v.mediaURL(ctx, item.UpAvatar)
		out = append(out, item)
	}
	return out
}

// getZoneCache 获取指定分区的缓存结构（懒创建，sync.Map 保证并发安全）
func (v *VideoLogic) getZoneCache(zone, cursor string, limit int) *zoneCache {
	key := fmt.Sprintf("%s|%s|%d", zone, cursor, limit)
	val, _ := v.zoneCaches.LoadOrStore(key, &zoneCache{})
	return val.(*zoneCache)
}

// ListHotVideos 获取已发布视频列表（Redis 优先，降级 MySQL）
func (v *VideoLogic) ListHotVideos(req request.HomeVideoList) ([]httpx.HomeVideoInfo, int64, string) {
	// 使用带超时的 context，防止 Redis/MySQL 慢查询导致 goroutine 长期阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	limit := req.Limit
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	zc := v.getZoneCache(req.Zone, req.Cursor, limit)

	// 第一次检查（无锁快速路径）：缓存有效时直接返回，避免锁开销
	if zc.result != nil && time.Now().Before(zc.expireAt) {
		v.cleanupExpiredZoneCaches()
		return v.signHomeList(ctx, zc.result.list), zc.result.total, zc.result.nextCursor
	}

	// 缓存未命中：加锁防止同一分页的并发请求同时穿透到 Redis/MySQL
	zc.mu.Lock()
	defer zc.mu.Unlock()

	// 第二次检查（持锁后）：可能其他 goroutine 已经在锁内回填了缓存
	if zc.result != nil && time.Now().Before(zc.expireAt) {
		return v.signHomeList(ctx, zc.result.list), zc.result.total, zc.result.nextCursor
	}

	list, total, cursor := v.redisPath(ctx, req)

	// 缓存结果 5 秒（缓存内保留对象 key，签名在响应阶段完成）
	zc.result = &hotVideosCache{list: list, total: total, nextCursor: cursor}
	zc.expireAt = time.Now().Add(5 * time.Second)

	return v.signHomeList(ctx, list), total, cursor
}

// redisPath Redis 缓存路径：ZSet 游标分页 → 批量查缓存 → 回填缺失
func (v *VideoLogic) redisPath(ctx context.Context, req request.HomeVideoList) ([]httpx.HomeVideoInfo, int64, string) {
	limit := int64(req.Limit)
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	// 根据分区构建 ZSet 键：zone 为空时使用全局 key，非空时使用分区 key
	zsetKey := v.deps.RedisClient.BuildPublishedZSetKey(req.Zone)

	cursorScore, cursorID := util.DecodeCursor(req.Cursor)
	cursorMember := ""
	if cursorID > 0 {
		cursorMember = strconv.FormatUint(uint64(cursorID), 10)
	}
	var zMembers []redis.Z
	var nextScore float64
	var nextCursorMember string
	zMembers, nextScore, nextCursorMember, zsetErr := v.deps.RedisClient.ZSetCursorPaginate(ctx, zsetKey, cursorScore, cursorMember, limit)
	if zsetErr != nil || len(zMembers) == 0 {
		if zsetErr != nil {
			v.deps.Logger.Error("Redis ZSet 游标分页失败，降级 MySQL", zap.Error(zsetErr))
		} else {
			// ZSet 为空（键不存在或尚未被定时任务填充）时也应降级到 MySQL
			v.deps.Logger.Warn("ZSet 为空，降级到 MySQL 查询", zap.String("zsetKey", zsetKey))
		}
		list, total, cursor, fallbackErr := v.mysqlFallback(ctx, req)
		if fallbackErr != nil {
			return nil, 0, ""
		}
		return list, total, cursor
	}

	// ZCard 查询总数，失败时 total 默认 0（非关键数据，不影响分页）
	var total int64
	if n, zcardErr := v.deps.RedisClient.ZCard(ctx, zsetKey); zcardErr != nil {
		v.deps.Logger.Warn("ZCard 查询失败", zap.String("key", zsetKey), zap.Error(zcardErr))
	} else {
		total = n
	}

	videoIDs := make([]uint, 0, len(zMembers))
	for _, z := range zMembers {
		memberStr, ok := z.Member.(string)
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseUint(memberStr, 10, 64)
		if parseErr != nil {
			continue
		}
		videoIDs = append(videoIDs, uint(id))
	}

	if len(videoIDs) == 0 {
		return nil, 0, ""
	}

	// 批量查 Redis 缓存；Redis 失败时将所有请求的 videoIDs 视为未命中，强制走 MySQL 回源
	cacheMap, missedIDs, redisErr := v.deps.VideoCacheRepo.GetVideoCache(ctx, videoIDs)
	if redisErr != nil || cacheMap == nil {
		missedIDs = videoIDs
		cacheMap = make(map[uint]*cache.VideoCacheData)
	}

	// 缓存未命中的视频从 MySQL 回源并回填 Redis（作者信息经 user-svc gRPC 补全）
	if len(missedIDs) > 0 {
		dbVideos, queryErr := v.deps.VideoRepo.FindPublishedVideosByIDs(ctx, missedIDs)
		if queryErr != nil {
			v.deps.Logger.Error("MySQL 回源查询失败，跳过缓存回填", zap.Error(queryErr))
		} else {
			v.deps.BackfillRepo.BackfillVideoCache(ctx, dbVideos, missedIDs, cacheMap)
		}
	}

	// 组装响应列表
	list := make([]httpx.HomeVideoInfo, 0, len(videoIDs))
	for _, vid := range videoIDs {
		data, ok := cacheMap[vid]
		if !ok || data.IsEmpty {
			continue
		}

		var createdAt time.Time
		if !data.CreatedAt.IsZero() {
			createdAt = data.CreatedAt
		}

		list = append(list, httpx.HomeVideoInfo{
			ID:           vid,
			UpName:       data.AuthorName,
			UpAvatar:     data.AuthorAvatar,
			Title:        data.Title,
			CoverURL:     data.CoverURL,
			PlayCount:    data.PlayCount,
			CommentCount: data.CommentCnt,
			Duration:     data.Duration,
			CreatedAt:    createdAt,
		})
	}

	nextCursorID := uint(0)
	if nextCursorMember != "" {
		if parsed, parseErr := strconv.ParseUint(nextCursorMember, 10, 64); parseErr == nil {
			nextCursorID = uint(parsed)
		}
	}
	return list, total, util.EncodeCursor(nextScore, nextCursorID)
}

// mysqlFallback MySQL 降级路径：直接查数据库，不走缓存
func (v *VideoLogic) mysqlFallback(ctx context.Context, req request.HomeVideoList) ([]httpx.HomeVideoInfo, int64, string, error) {
	var query = v.deps.VideoRepo.FindPublishedVideos()
	if req.Zone != "" {
		query = v.deps.VideoRepo.FindPublishedVideosWithZone(req.Zone)
	}

	opt := CursorOption{
		CursorPage: request.CursorPage{
			Limit:  req.Limit,
			Cursor: req.Cursor,
		},
		Fields: []CursorField{
			{Column: "popularity", Direction: "DESC"},
			{Column: "id", Direction: "DESC"},
		},
	}

	var videos []model.Video
	total, nextCursor, err := v.deps.PaginateRepo.CursorPaginate(ctx, query, opt, &videos)
	if err != nil {
		v.deps.Logger.Error("MySQL 游标分页失败", zap.Error(err))
		return nil, 0, "", err
	}

	// 作者名/头像属于 user-svc，走 gRPC 批量补全（原实现查 accounts 表）
	seen := make(map[string]bool)
	var authorIDs []string
	for _, vid := range videos {
		if vid.AuthorID == "" || seen[vid.AuthorID] {
			continue
		}
		seen[vid.AuthorID] = true
		authorIDs = append(authorIDs, vid.AuthorID)
	}
	userMap := v.deps.BackfillRepo.LookupUsers(ctx, authorIDs)

	list := make([]httpx.HomeVideoInfo, 0, len(videos))
	for _, vid := range videos {
		var name, avatar string
		if u, ok := userMap[vid.AuthorID]; ok {
			name, avatar = u.Username, u.AvatarUrl
		}
		list = append(list, httpx.HomeVideoInfo{
			ID:           vid.ID,
			UpName:       name,
			UpAvatar:     avatar,
			Title:        vid.Title,
			CoverURL:     vid.CoverURL,
			PlayCount:    vid.PlayCount,
			CommentCount: vid.CommentsCount,
			Duration:     vid.DurationSec,
			CreatedAt:    vid.CreatedAt,
		})
	}
	return list, total, nextCursor, nil
}

// GetVideoDetail 获取视频详情（Redis 缓存优先，未命中降级 MySQL 并回填）
func (v *VideoLogic) GetVideoDetail(ctx context.Context, userID string, videoID uint) (*httpx.VideoDetailResp, error) {
	// ---- 1. 获取视频基本信息（缓存优先） ----
	videoData := v.getVideoDataWithFallback(ctx, videoID)
	if videoData == nil {
		return nil, fmt.Errorf("video not found")
	}

	// ---- 2. 获取作者信息（跨服务 gRPC，原实现读 accounts + 用户缓存） ----
	authorInfo := httpx.AuthorInfo{ID: videoData.AuthorID}
	if videoData.AuthorID != "" {
		u, userErr := v.deps.UserClient.GetUser(ctx, videoData.AuthorID)
		if userErr != nil {
			v.deps.Logger.Warn("GetUser via gRPC failed",
				zap.String("author_id", videoData.AuthorID), zap.Error(userErr))
		} else if u != nil {
			authorInfo.Username = u.Username
			authorInfo.AvatarURL = u.AvatarUrl
			authorInfo.Signature = u.Signature
			authorInfo.FansCount = u.FollowerCount
		}
	}

	// ---- 3. 获取用户互动状态（跨服务 gRPC，仅已登录用户） ----
	interaction := v.getInteraction(ctx, userID, videoID, videoData.AuthorID)

	// ---- 4. 播放量 +1（Redis 原子自增 + Kafka 异步同步 MySQL） ----
	if incrErr := v.deps.VideoCacheRepo.IncrementPlayCount(ctx, videoID); incrErr != nil {
		v.deps.Logger.Warn("increment play count failed", zap.Error(incrErr))
	}
	if err := v.deps.PlayCountPublisher.PublishIncrement(ctx, videoID, 1); err != nil {
		v.deps.Logger.Warn("publish play count increment failed", zap.Error(err))
	}

	// ---- 5. 组装响应 ----
	var createdAt time.Time
	if !videoData.CreatedAt.IsZero() {
		createdAt = videoData.CreatedAt
	}

	return &httpx.VideoDetailResp{
		ID:          videoID,
		Title:       videoData.Title,
		Description: videoData.Description,
		PlayURL:     v.mediaURL(ctx, videoData.PlayURL),
		CoverURL:    v.mediaURL(ctx, videoData.CoverURL),
		Duration:    videoData.Duration,
		Zone:        videoData.Zone,
		PlayCount:   videoData.PlayCount,
		LikesCnt:    videoData.LikesCnt,
		CommentCnt:  videoData.CommentCnt,
		CreatedAt:   createdAt,
		Author:      authorInfo,
		Interaction: interaction,
	}, nil
}

// getVideoDataWithFallback 获取视频缓存数据，未命中则降级查 MySQL 并回填
func (v *VideoLogic) getVideoDataWithFallback(ctx context.Context, videoID uint) *cache.VideoCacheData {
	// 第一次：无锁查缓存（快速路径）
	cacheMap, _, redisErr := v.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{videoID})
	if redisErr != nil {
		v.deps.Logger.Warn("Redis get video cache failed", zap.Uint("video_id", videoID), zap.Error(redisErr))
	}
	if data, ok := cacheMap[videoID]; ok && !data.IsEmpty {
		return data
	}

	// 缓存未命中：使用 singleflight 防击穿
	sfResult, sfErr, _ := v.sfGroup.Do(fmt.Sprintf("video:%d", videoID), func() (interface{}, error) {
		// singleflight 回调内再次查缓存（双重检查），可能其他 goroutine 已回填
		innerCacheMap, innerMissedIDs, innerErr := v.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{videoID})
		if innerErr != nil || innerCacheMap == nil {
			innerMissedIDs = []uint{videoID}
			innerCacheMap = make(map[uint]*cache.VideoCacheData)
		}
		if data, ok := innerCacheMap[videoID]; ok && !data.IsEmpty {
			return data, nil
		}

		// 缓存未命中，从 MySQL 回源
		if len(innerMissedIDs) > 0 {
			dbVideos, queryErr := v.deps.VideoRepo.FindPublishedVideosByIDs(ctx, innerMissedIDs)
			if queryErr != nil {
				return nil, queryErr
			}
			v.deps.BackfillRepo.BackfillVideoCache(ctx, dbVideos, innerMissedIDs, innerCacheMap)
			if data, ok := innerCacheMap[videoID]; ok && !data.IsEmpty {
				return data, nil
			}
		}
		return nil, nil
	})
	if sfErr != nil {
		v.deps.Logger.Warn("getVideoDataWithFallback failed",
			zap.Uint("video_id", videoID), zap.Error(sfErr))
		return nil
	}
	if sfResult != nil {
		return sfResult.(*cache.VideoCacheData)
	}
	return nil
}

// getInteraction 获取当前用户对视频的互动状态：
//   - 点赞状态来自 interaction-svc
//   - 关注状态来自 user-svc（关注关系在 user 库）
//
// 两个远程调用相互独立：任一失败只影响对应字段，不影响详情页整体返回。
func (v *VideoLogic) getInteraction(ctx context.Context, userID string, videoID uint, authorID string) httpx.InteractionStatusResp {
	if userID == "" {
		return httpx.InteractionStatusResp{}
	}

	resp := httpx.InteractionStatusResp{}

	state, err := v.deps.InteractionClient.GetVideoUserState(ctx, userID, []uint64{uint64(videoID)})
	if err != nil {
		v.deps.Logger.Warn("GetVideoUserState via gRPC failed",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
	} else {
		for _, id := range state.LikedVideoIds {
			if id == uint64(videoID) {
				resp.IsLiked = true
			}
		}
	}

	if authorID != "" && authorID != userID {
		followed, ferr := v.deps.UserClient.IsFollowing(ctx, userID, authorID)
		if ferr != nil {
			v.deps.Logger.Warn("IsFollowing via gRPC failed",
				zap.String("user_id", userID), zap.String("author_id", authorID), zap.Error(ferr))
		} else {
			resp.IsFollowed = followed
		}
	}

	return resp
}

// cleanupExpiredZoneCaches 清理已过期的 zoneCache 条目，防止 sync.Map 无限增长
func (v *VideoLogic) cleanupExpiredZoneCaches() {
	now := time.Now()
	v.zoneCaches.Range(func(key, value interface{}) bool {
		zc, ok := value.(*zoneCache)
		if !ok {
			return true
		}
		// 条目已过期且无人持有锁：安全删除
		if zc.result != nil && now.After(zc.expireAt) {
			v.zoneCaches.Delete(key)
		}
		return true
	})
}
