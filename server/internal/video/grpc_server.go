package video

import (
	"context"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/storagex"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VideoGRPCServer video-svc 的 gRPC 服务实现。
//
// 供 user-svc（主页视频列表/统计）与 interaction-svc（校验视频存在、取作者）使用。
// DB 中保存的是对象 key，gRPC 响应在序列化时统一转成可访问 URL，
// 下游服务（interaction）无需引入存储依赖。
type VideoGRPCServer struct {
	pb.UnimplementedVideoServiceServer
	videoRepo *VideoRepo
	storage   storagex.Storage
}

func NewVideoGRPCServer(videoRepo *VideoRepo, storage storagex.Storage) *VideoGRPCServer {
	return &VideoGRPCServer{videoRepo: videoRepo, storage: storage}
}

func (s *VideoGRPCServer) GetVideo(ctx context.Context, req *pb.GetVideoReq) (*pb.VideoBrief, error) {
	// 跨服务只暴露已发布视频：转码中的视频不应该被点赞或评论
	v, err := s.videoRepo.FindPublishedVideoByID(ctx, uint(req.VideoId))
	if err != nil {
		return nil, status.Error(codes.NotFound, "video not found")
	}
	return s.videoToBrief(ctx, v), nil
}

func (s *VideoGRPCServer) GetVideos(ctx context.Context, req *pb.GetVideosReq) (*pb.GetVideosResp, error) {
	ids := make([]uint, 0, len(req.VideoIds))
	for _, id := range req.VideoIds {
		ids = append(ids, uint(id))
	}
	videos, err := s.videoRepo.FindPublishedVideosByIDs(ctx, ids)
	if err != nil {
		return nil, status.Error(codes.Internal, "query videos failed")
	}
	resp := &pb.GetVideosResp{Videos: make([]*pb.VideoBrief, 0, len(videos))}
	for i := range videos {
		resp.Videos = append(resp.Videos, s.videoToBrief(ctx, &videos[i]))
	}
	return resp, nil
}

func (s *VideoGRPCServer) ListUserVideos(ctx context.Context, req *pb.ListUserVideosReq) (*pb.VideoListResp, error) {
	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 || pageSize > 30 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	videos, err := s.videoRepo.FindPublishedVideosByAuthorID(ctx, req.UserId, pageSize, offset)
	if err != nil {
		return nil, status.Error(codes.Internal, "query videos failed")
	}
	total, err := s.videoRepo.CountPublishedByAuthorID(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "count videos failed")
	}

	resp := &pb.VideoListResp{
		Videos: make([]*pb.VideoBrief, 0, len(videos)),
		Total:  total,
	}
	for i := range videos {
		resp.Videos = append(resp.Videos, s.videoToBrief(ctx, &videos[i]))
	}
	return resp, nil
}

func (s *VideoGRPCServer) GetUserStats(ctx context.Context, req *pb.GetUserStatsReq) (*pb.UserStats, error) {
	videoCount, playCount, likeCount, err := s.videoRepo.SumPublishedStatsByAuthorID(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "aggregate stats failed")
	}
	return &pb.UserStats{
		VideoCount:     videoCount,
		TotalPlayCount: playCount,
		TotalLikeCount: likeCount,
	}, nil
}

func (s *VideoGRPCServer) IsVideoOwner(ctx context.Context, req *pb.IsVideoOwnerReq) (*pb.BoolResp, error) {
	v, err := s.videoRepo.FindPublishedVideoByID(ctx, uint(req.VideoId))
	if err != nil {
		return &pb.BoolResp{Ok: false}, nil
	}
	return &pb.BoolResp{Ok: v.AuthorID == req.UserId}, nil
}

// videoToBrief 把 model.Video 转换为 gRPC 传输结构，媒体字段输出已签名 URL。
func (s *VideoGRPCServer) videoToBrief(ctx context.Context, v *model.Video) *pb.VideoBrief {
	coverURL, _ := s.storage.URL(ctx, v.CoverURL)
	playURL, _ := s.storage.URL(ctx, v.PlayURL)
	return &pb.VideoBrief{
		Id:            uint64(v.ID),
		AuthorId:      v.AuthorID,
		Title:         v.Title,
		Status:        v.Status,
		CoverUrl:      coverURL,
		PlayUrl:       playURL,
		Duration:      v.DurationSec,
		PlayCount:     v.PlayCount,
		LikeCount:     v.LikesCount,
		CommentCount:  v.CommentsCount,
		CreatedAtUnix: v.CreatedAt.Unix(),
		Description:   v.Description,
	}
}
