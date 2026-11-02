package user

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/auth"
	"fake_tiktok/pkg/httpx"
	"fake_tiktok/pkg/otelx"
	"fake_tiktok/pkg/storagex"
	"fake_tiktok/pkg/util"

	"github.com/bwmarrin/snowflake"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// -----------------------------------------------------------------------------

type UserLogic struct {
	deps *LogicDeps
	// sfGroup 用于缓存击穿保护：同一个 userID 的并发回源请求只执行一次，
	// 其他请求共享结果，避免大量并发请求同时穿透到 MySQL
	sfGroup singleflight.Group
}

func NewUserLogic(deps *LogicDeps) *UserLogic {
	return &UserLogic{deps: deps}
}

// -----------------------------------------------------------------------------

type TokenResult struct {
	Account           model.Account
	AccessToken       string
	AccessTokenExpire int64
	RefreshToken      string
	RefreshExpiry     int
}

var snowNode *snowflake.Node

// snowOnce 保证 snowNode 只初始化一次，避免并发调用时重复创建或竞态访问。
var snowOnce sync.Once

// ensureSnowNode 确保 snowNode 已初始化（懒初始化 + 并发安全）。
func ensureSnowNode() {
	snowOnce.Do(func() {
		node, err := snowflake.NewNode(1)
		if err != nil {
			panic(fmt.Sprintf("failed to create snowflake node: %v", err))
		}
		snowNode = node
	})
}

// -----------------------------------------------------------------------------

func (u *UserLogic) Register(ctx context.Context, account model.Account) (model.Account, error) {
	if _, err := u.deps.AccountRepo.FindByUsername(ctx, account.Username); err != nil {
		// 非 ErrRecordNotFound 的错误（如 DB 连接失败）不应被静默忽略，
		// 否则可能导致重复用户名注册
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Account{}, fmt.Errorf("check username failed: %w", err)
		}
		// ErrRecordNotFound: 用户名未注册，继续注册流程
	} else {
		return model.Account{}, errors.New("username already exists")
	}
	ensureSnowNode()
	hashedPwd, err := util.BcryptHash(account.Password)
	if err != nil {
		return model.Account{}, fmt.Errorf("failed to hash password: %w", err)
	}
	account.Password = hashedPwd
	account.ID = snowNode.Generate().String()
	// 头像为空时前端统一 fallback 到 /images/avatar-default.jpg
	account.AvatarURL = ""
	if err := u.deps.AccountRepo.Create(ctx, &account); err != nil {
		return model.Account{}, err
	}
	return account, nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) Login(ctx context.Context, account model.Account) (model.Account, error) {
	// 手动埋点：登录是用户维度排查问题的高频入口，失败原因多（账号不存在/密码错/DB 错）。
	ctx, span := otelx.Tracer("user-svc").Start(ctx, "user.login")
	defer span.End()

	user, err := u.deps.AccountRepo.FindByUsername(ctx, account.Username)
	if err == nil {
		if !util.BcryptCheck(account.Password, user.Password) {
			return model.Account{}, errors.New("password error")
		}
		// 登录成功后才拿得到 user_id，此时补写属性
		span.SetAttributes(attribute.String("user_id", user.ID))
		// DB 存对象 key，登录响应需要转成可访问地址
		user.AvatarURL = u.avatarURL(ctx, user.AvatarURL)
		return *user, nil
	}
	return model.Account{}, errors.New("user not found")
}

// -----------------------------------------------------------------------------

// getUserCacheData 获取用户缓存数据（Redis 优先，singleflight 防击穿 + StaticHit 降级回源）
// 供 PersonalInfo 和 UserHome 复用
func (u *UserLogic) getUserCacheData(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	var userData *cache.UserCacheData
	if data, err := u.deps.UserCacheRepo.GetUserCache(ctx, userID); err == nil {
		userData = data
	}
	if userData == nil {
		// 全部未命中：走 MySQL 回源（static + dynamic 一起回填）
		v, sfErr, _ := u.sfGroup.Do("user:"+userID, func() (interface{}, error) {
			return u.deps.BackfillRepo.BackfillUserCache(ctx, userID)
		})
		if sfErr != nil {
			u.deps.Logger.Warn("BackfillUserCache failed", zap.String("user_id", userID), zap.Error(sfErr))
		} else if v != nil {
			userData = v.(*cache.UserCacheData)
		}
	} else if !userData.StaticHit {
		// 动态区命中但静态区未命中：只对 static 区做降级回源
		v, sfErr, _ := u.sfGroup.Do("user:static:"+userID, func() (interface{}, error) {
			return u.deps.BackfillRepo.BackfillUserCache(ctx, userID)
		})
		if sfErr != nil {
			u.deps.Logger.Warn("BackfillUserCache static failed", zap.String("user_id", userID), zap.Error(sfErr))
		} else if v != nil {
			backfilled := v.(*cache.UserCacheData)
			userData.ID = backfilled.ID
			userData.Username = backfilled.Username
			userData.AvatarURL = backfilled.AvatarURL
			userData.Signature = backfilled.Signature
			userData.Birthday = backfilled.Birthday
			userData.Gender = backfilled.Gender
			userData.StaticHit = true
		}
	}
	return userData, nil
}

// UserHome 获取用户主页信息
// viewerID 为当前登录用户（用于计算 is_followed）；未登录时传空串。
// page / pageSize 用于控制作者视频列表的分页，来自请求参数
func (u *UserLogic) UserHome(ctx context.Context, viewerID, userID string, page, pageSize int) (*httpx.UserHomeResp, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// ---- 1. 获取用户缓存 ----
	userData, err := u.getUserCacheData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// ---- 2. 作者视频列表（按时间倒序，使用请求分页参数） ----
	var videos []httpx.HomeVideoInfo
	// 头像 DB 存 key，响应前转成可访问地址
	avatarURL := u.avatarURL(ctx, userData.AvatarURL)
	// 分页参数校验：page 从 1 开始，pageSize 默认 10、上限 30
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 30 {
		pageSize = 10
	}
	// 视频库已独立为 video-svc，只能通过 gRPC 拉取作者视频列表
	videoResp, verr := u.deps.VideoClient.ListUserVideos(ctx, userID, page, pageSize)
	if verr != nil {
		u.deps.Logger.Warn("ListUserVideos failed", zap.String("user_id", userID), zap.Error(verr))
	} else {
		videos = make([]httpx.HomeVideoInfo, 0, len(videoResp.Videos))
		for _, v := range videoResp.Videos {
			videos = append(videos, briefToHomeVideoInfo(v, userData.Username, avatarURL))
		}
	}

	// ---- 4. 组装响应 ----
	// 用户维度统计按需从视频库聚合（gRPC GetUserStats）：
	// 视频数 COUNT、播放量 SUM(play_count)、获赞 SUM(likes_count)。
	var realVideoCount, realPlayCount, realLikesReceived int64
	if stats, statsErr := u.deps.VideoClient.GetUserStats(ctx, userID); statsErr == nil && stats != nil {
		realVideoCount, realPlayCount, realLikesReceived = stats.VideoCount, stats.TotalPlayCount, stats.TotalLikeCount
	} else {
		u.deps.Logger.Warn("GetUserStats failed", zap.String("user_id", userID), zap.Error(statsErr))
	}
	// fans_count / following_count 实时从本服务 user_follows 表统计。
	realFansCount := userData.FansCount
	if n, ferr := u.deps.FollowRepo.GetFansCount(ctx, userID); ferr == nil {
		realFansCount = n
	}
	realFollowingCount := userData.FollowingCount
	if n, ferr := u.deps.FollowRepo.GetFollowingCount(ctx, userID); ferr == nil {
		realFollowingCount = n
	}

	// is_followed：当前登录用户是否关注了该主页用户。
	// 仅当 viewer 存在且不是本人时才查询；否则（未登录 / 自己主页）为 false。
	isFollowed := false
	if viewerID != "" && viewerID != userID {
		if ok, ferr := u.deps.FollowRepo.IsUserFollowed(ctx, viewerID, userID); ferr == nil {
			isFollowed = ok
		}
	}

	return &httpx.UserHomeResp{
		ID:                 userData.ID,
		AvatarURL:          avatarURL,
		Signature:          userData.Signature,
		Username:           userData.Username,
		VideoCount:         realVideoCount,
		Birthday:           userData.Birthday,
		Gender:             userData.Gender,
		TotalLikesReceived: realLikesReceived,
		TotalPlayCount:     realPlayCount,
		FansCount:          realFansCount,
		FollowingCount:     realFollowingCount,
		IsFollowed:         isFollowed,
		Videos:             videos,
	}, nil
}

// briefToHomeVideoInfo 把 video-svc 的 VideoBrief 转换为首页视频卡片结构。
// 字段名必须与拆服务前保持完全一致（前端依赖 JSON 字段）。
func briefToHomeVideoInfo(v *pb.VideoBrief, upName, upAvatar string) httpx.HomeVideoInfo {
	var createdAt time.Time
	if v.CreatedAtUnix > 0 {
		createdAt = time.Unix(v.CreatedAtUnix, 0)
	}
	return httpx.HomeVideoInfo{
		ID:           uint(v.Id),
		UpName:       upName,
		UpAvatar:     upAvatar,
		Title:        v.Title,
		CoverURL:     v.CoverUrl,
		PlayCount:    v.PlayCount,
		CommentCount: v.CommentCount,
		Duration:     v.Duration,
		CreatedAt:    createdAt,
	}
}

// -----------------------------------------------------------------------------

// Logout 用户登出：删除 Redis 中的 JWT 记录 + 将 refresh token 加入黑名单。
func (u *UserLogic) Logout(userID string, refreshToken string) error {
	var errs []error

	if err := u.deps.JWTRepo.DelJWT(context.Background(), userID); err != nil {
		u.deps.Logger.Warn("Logout: failed to delete JWT record",
			zap.String("user_id", userID), zap.Error(err))
		errs = append(errs, fmt.Errorf("delete JWT: %w", err))
	}

	refreshExpiry, _ := util.ParseDuration(u.deps.Config.JWT.RefreshTokenExpiryTime)
	if err := u.deps.JWTRepo.ToBlackList(context.Background(), refreshToken, refreshExpiry); err != nil {
		u.deps.Logger.Warn("Logout: failed to blacklist refresh token",
			zap.String("user_id", userID), zap.Error(err))
		errs = append(errs, fmt.Errorf("blacklist refresh token: %w", err))
	}
	return errors.Join(errs...)
}

// -----------------------------------------------------------------------------

// UserInfo 获取当前登录用户的个人信息，走 Redis 缓存（与 UserHome 共用缓存逻辑）
func (u *UserLogic) UserInfo(ctx context.Context, userID string) (*httpx.UserHomeResp, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	userData, err := u.getUserCacheData(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// 统计按需从视频库聚合（gRPC），与 UserHome 保持一致
	var videoCount, playCount, likesReceived int64
	if stats, statsErr := u.deps.VideoClient.GetUserStats(ctx, userID); statsErr == nil && stats != nil {
		videoCount, playCount, likesReceived = stats.VideoCount, stats.TotalPlayCount, stats.TotalLikeCount
	} else {
		u.deps.Logger.Warn("GetUserStats failed", zap.String("user_id", userID), zap.Error(statsErr))
	}
	return &httpx.UserHomeResp{
		ID:                 userData.ID,
		AvatarURL:          u.avatarURL(ctx, userData.AvatarURL),
		Signature:          userData.Signature,
		Username:           userData.Username,
		VideoCount:         videoCount,
		Birthday:           userData.Birthday,
		Gender:             userData.Gender,
		TotalLikesReceived: likesReceived,
		TotalPlayCount:     playCount,
		FansCount:          userData.FansCount,
		FollowingCount:     userData.FollowingCount,
	}, nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) UserChangeInfo(ctx context.Context, req request.UserChangeInfo) error {
	if _, err := u.deps.AccountRepo.FindByID(ctx, req.UserID); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"username":  req.Username,
		"signature": req.Signature,
		"birthday":  req.Birthday,
		"gender":    req.Gender,
	}
	// 仅在前端传了非空 avatar 时才更新，避免用空串覆盖已有头像 key
	if req.Avatar != "" {
		// 前端回传的是上传接口返回的可访问地址，归一化为对象 key 后入库
		updates["avatar_url"] = storagex.KeyFromURL(req.Avatar, u.deps.Config.Minio.Bucket)
	}
	if err := u.deps.AccountRepo.Updates(ctx, req.UserID, updates); err != nil {
		return err
	}
	// 静态信息变更后只删除静态区缓存，动态区缓存（计数类）不受影响
	if err := u.deps.UserCacheRepo.DeleteUserCache(ctx, req.UserID); err != nil {
		u.deps.Logger.Error("删除用户缓存失败，可能存在数据不一致",
			zap.String("userID", req.UserID), zap.Error(err))
	}
	return nil
}

// -----------------------------------------------------------------------------

func (u *UserLogic) VerifyCaptcha(captchaID, captcha string) bool {
	// 内网/演示环境可通过 APP_SKIP_CAPTCHA 跳过校验（配置项 server.skip_captcha）
	if u.deps.Config.Server.SkipCaptcha {
		return true
	}
	return u.deps.CaptchaStore.Verify(captchaID, captcha, true)
}

// -----------------------------------------------------------------------------

func (u *UserLogic) GenerateToken(user model.Account) (*TokenResult, error) {
	j := auth.NewJWT(&u.deps.Config.JWT)

	baseClaims := request.BaseClaims{
		UserID: user.ID,
	}

	accessClaims, err := j.CreateAccessClaims(baseClaims, &u.deps.Config.JWT)
	if err != nil {
		return nil, err
	}

	accessToken, err := j.CreateAccessToken(accessClaims)
	if err != nil {
		return nil, err
	}

	refreshClaims, err := j.CreateRefreshClaims(baseClaims, &u.deps.Config.JWT)
	if err != nil {
		return nil, err
	}

	refreshToken, err := j.CreateRefreshToken(refreshClaims)
	if err != nil {
		return nil, err
	}

	oldToken, err := u.deps.JWTRepo.GetJWT(context.Background(), user.ID)
	if err == nil && oldToken != "" {
		refreshExpiry, _ := util.ParseDuration(u.deps.Config.JWT.RefreshTokenExpiryTime)
		_ = u.deps.JWTRepo.ToBlackList(context.Background(), oldToken, refreshExpiry)
	}

	expiry := time.Until(refreshClaims.ExpiresAt.Time)
	if err := u.deps.JWTRepo.SetJWT(context.Background(), user.ID, refreshToken, expiry); err != nil {
		u.deps.Logger.Error("存储 Refresh Token 到 Redis 失败",
			zap.String("userID", user.ID), zap.Error(err))
	}

	refreshExpiry := int(refreshClaims.ExpiresAt.Unix() - time.Now().Unix())

	return &TokenResult{
		Account:           user,
		AccessToken:       accessToken,
		AccessTokenExpire: accessClaims.ExpiresAt.Unix() * 1000,
		RefreshToken:      refreshToken,
		RefreshExpiry:     refreshExpiry,
	}, nil
}

// -----------------------------------------------------------------------------

var allowedImageExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

const (
	maxFileSize = 2097152
)

// -----------------------------------------------------------------------------

// UploadAvatar 上传用户头像，采用"写临时文件 + 原子 rename"模式（由 storagex 统一实现）。
func (u *UserLogic) UploadAvatar(ctx context.Context, userID string, file *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedImageExt[ext] {
		return "", errors.New("unsupported image format, only jpg/jpeg/png/gif/webp allowed")
	}

	if file.Size > maxFileSize {
		return "", fmt.Errorf("file size exceeds limit (%d bytes)", maxFileSize)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// 通过统一对象存储上传（DB 只保存对象 key）。
	key := fmt.Sprintf("avatar/%s%s", userID, ext)
	if err := u.deps.Storage.Put(ctx, key, src, file.Size); err != nil {
		return "", fmt.Errorf("failed to save avatar: %w", err)
	}

	if err := u.deps.AccountRepo.UpdateColumn(ctx, userID, "avatar_url", key); err != nil {
		// 数据库更新失败：回滚已上传的头像对象，避免存储与 DB 不一致。
		_ = u.deps.Storage.Delete(ctx, key)
		return "", fmt.Errorf("failed to update avatar url: %w", err)
	}

	// 响应返回可访问地址（公开直链或预签名）
	return u.avatarURL(ctx, key), nil
}

// avatarURL 把头像对象 key 转成可访问地址；空 key 返回空串（前端统一兜底）。
func (u *UserLogic) avatarURL(ctx context.Context, key string) string {
	return mediaURL(ctx, u.deps, key)
}

// mediaURL 把对象 key 转成可访问地址；已是绝对 URL 的旧值原样返回。
func mediaURL(ctx context.Context, deps *LogicDeps, key string) string {
	if key == "" {
		return ""
	}
	url, err := deps.Storage.URL(ctx, key)
	if err != nil {
		deps.Logger.Warn("sign media url failed", zap.String("key", key), zap.Error(err))
		return ""
	}
	return url
}
