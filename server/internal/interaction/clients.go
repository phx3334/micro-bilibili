package interaction

import (
	"context"
	"time"

	"fake_tiktok/api/pb"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// VideoGRPCClient 调用 video-svc 的 gRPC 客户端（懒连接，启动不依赖下游）。
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

// GetVideo 拉取单个视频信息（校验存在性、评论区开关、作者 ID）。
func (c *VideoGRPCClient) GetVideo(ctx context.Context, videoID uint64) (*pb.VideoBrief, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.GetVideo(ctx, &pb.GetVideoReq{VideoId: videoID})
}

// UserGRPCClient 调用 user-svc 的 gRPC 客户端。
type UserGRPCClient struct {
	client pb.UserServiceClient
}

func NewUserGRPCClient(addr string) (*UserGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, err
	}
	return &UserGRPCClient{client: pb.NewUserServiceClient(conn)}, nil
}

// GetUsers 批量拉取评论者/作者的用户名+头像，返回 userID → UserInfo 映射。
func (c *UserGRPCClient) GetUsers(ctx context.Context, userIDs []string) (map[string]*pb.UserInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	resp, err := c.client.GetUsers(ctx, &pb.GetUsersReq{UserIds: userIDs})
	if err != nil {
		return nil, err
	}
	out := make(map[string]*pb.UserInfo, len(resp.Users))
	for _, u := range resp.Users {
		out[u.Id] = u
	}
	return out, nil
}
