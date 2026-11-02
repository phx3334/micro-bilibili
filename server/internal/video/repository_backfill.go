package video

import (
	"context"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/storagex"

	"go.uber.org/zap"
)

// BackfillRepo 视频缓存回源。
//
// 作者用户名/头像属于 user-svc 的 feed_user 库，拆分后不再查 accounts 表，
// 改为 gRPC GetUsers 批量拉取；视频本身仍从本地 feed_video 库回源。
// 缓存中只保存对象 key：作者头像 URL 会反解成 key，读取时再重新签名。
type BackfillRepo struct {
	videoCacheRepo *VideoCacheRepo
	userClient     *UserGRPCClient
	logger         *zap.Logger
	minioBucket    string
}

func NewBackfillRepo(videoCacheRepo *VideoCacheRepo, userClient *UserGRPCClient, logger *zap.Logger, minioBucket string) *BackfillRepo {
	return &BackfillRepo{
		videoCacheRepo: videoCacheRepo,
		userClient:     userClient,
		logger:         logger,
		minioBucket:    minioBucket,
	}
}

// LookupUsers 批量查询作者的用户名/头像（gRPC）。
// 失败时返回空 map，不阻塞业务返回。
func (r *BackfillRepo) LookupUsers(ctx context.Context, authorIDs []string) map[string]*pb.UserInfo {
	if len(authorIDs) == 0 {
		return make(map[string]*pb.UserInfo)
	}
	users, err := r.userClient.GetUsers(ctx, authorIDs)
	if err != nil {
		r.logger.Warn("LookupUsers via gRPC failed", zap.Error(err))
		return make(map[string]*pb.UserInfo)
	}
	return users
}

// BackfillVideoCache 从 MySQL 回源视频信息并回填 Redis 缓存
func (r *BackfillRepo) BackfillVideoCache(ctx context.Context, dbVideos []model.Video, missedIDs []uint, cacheMap map[uint]*cache.VideoCacheData) {
	dbMap := make(map[uint]model.Video, len(dbVideos))
	for _, vid := range dbVideos {
		dbMap[vid.ID] = vid
	}

	// 收集缺失的作者 ID，批量查询作者名+头像（跨服务 gRPC）
	authorIDs := r.collectMissingAuthorIDs(dbMap, missedIDs)
	userMap := r.LookupUsers(ctx, authorIDs)

	var writeItems []*cache.VideoCacheData
	var emptyIDs []uint

	for _, vid := range missedIDs {
		dbVid, ok := dbMap[vid]
		if !ok {
			// MySQL 中也不存在，标记为空对象防止缓存穿透
			emptyIDs = append(emptyIDs, vid)
			cacheMap[vid] = &cache.VideoCacheData{IsEmpty: true}
			continue
		}

		var name, avatarKey string
		if u, ok := userMap[dbVid.AuthorID]; ok {
			name = u.Username
			// gRPC 返回的是可访问 URL，缓存只保留对象 key（预签名 URL 有时效）
			avatarKey = storagex.KeyFromURL(u.AvatarUrl, r.minioBucket)
		}
		cacheMap[vid] = cache.VideoToCacheData(&dbVid, name, avatarKey)
		writeItems = append(writeItems, cache.VideoToCacheData(&dbVid, name, avatarKey))
	}

	r.videoCacheRepo.WriteVideoCache(ctx, writeItems, emptyIDs)
}

// collectMissingAuthorIDs 从缺失的视频 ID 中收集不重复的作者 ID
func (r *BackfillRepo) collectMissingAuthorIDs(dbMap map[uint]model.Video, missedIDs []uint) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, vid := range missedIDs {
		dbVid, ok := dbMap[vid]
		if !ok || dbVid.AuthorID == "" {
			continue
		}
		if seen[dbVid.AuthorID] {
			continue
		}
		seen[dbVid.AuthorID] = true
		ids = append(ids, dbVid.AuthorID)
	}
	return ids
}
