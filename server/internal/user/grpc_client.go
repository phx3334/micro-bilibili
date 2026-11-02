package user

import (
	"context"
	"time"

	"fake_tiktok/api/pb"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// VideoGRPCClient 调用 video-svc 的 gRPC 客户端。
//
// 使用 grpc.NewClient 懒连接：构造时不做网络操作，
// 启动顺序不依赖 video-svc 已就绪；真正调用时才建立连接。
type VideoGRPCClient struct {
	client pb.VideoServiceClient
}

func NewVideoGRPCClient(addr string) (*VideoGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, err
	}
	return &VideoGRPCClient{client: pb.NewVideoServiceClient(conn)}, nil
}

// ListUserVideos 拉取某作者的已发布视频列表（分页）。
// 原 UserHomeLogic 直接查 VideoRepo，拆库后改走 video-svc。
func (c *VideoGRPCClient) ListUserVideos(ctx context.Context, userID string, page, pageSize int) (*pb.VideoListResp, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.ListUserVideos(ctx, &pb.ListUserVideosReq{
		UserId:   userID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
}

// GetUserStats 拉取作者视频数 / 总播放 / 总获赞。
// 原 UserLogic 直接调 SumPublishedStatsByAuthorID，拆库后改走 video-svc。
func (c *VideoGRPCClient) GetUserStats(ctx context.Context, userID string) (*pb.UserStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.GetUserStats(ctx, &pb.GetUserStatsReq{UserId: userID})
}
