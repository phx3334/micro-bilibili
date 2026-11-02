package user

import (
	"context"
	"fmt"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/httpx"
)

// UserHomeLogic 用户主页相关业务逻辑
type UserHomeLogic struct {
	deps *LogicDeps
}

func NewUserHomeLogic(deps *LogicDeps) *UserHomeLogic {
	return &UserHomeLogic{deps: deps}
}

// ListUserVideos 用户主页视频列表（按 CreatedAt 倒序）
//
// 视频数据在 video-svc 的 feed_video 库，拆库后改为 gRPC ListUserVideos；
// 作者信息仍在本服务库内，直接用 AccountRepo，避免多一次远程调用。
func (l *UserHomeLogic) ListUserVideos(ctx context.Context, req request.ListUserVideosReq) (*httpx.PaginatedResp[httpx.HomeVideoInfo], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	resp, err := l.deps.VideoClient.ListUserVideos(ctx, req.UserID, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询视频列表失败")
	}

	// 获取作者信息（用户名、头像）
	var authorName, authorAvatar string
	if account, accErr := l.deps.AccountRepo.FindByID(ctx, req.UserID); accErr == nil {
		authorName = account.Username
		// DB 存对象 key，响应时转成可访问地址
		authorAvatar = mediaURL(ctx, l.deps, account.AvatarURL)
	}

	items := make([]httpx.HomeVideoInfo, 0, len(resp.Videos))
	for _, v := range resp.Videos {
		items = append(items, briefToHomeVideoInfo(v, authorName, authorAvatar))
	}

	return &httpx.PaginatedResp[httpx.HomeVideoInfo]{
		List:     items,
		Total:    resp.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
