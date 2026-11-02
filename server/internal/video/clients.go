package video

import (
	"context"
	"time"

	"fake_tiktok/api/pb"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// UserGRPCClient 调用 user-svc 的 gRPC 客户端。
//
// 使用 grpc.NewClient 懒连接：构造时不建连，video-svc 启动不依赖 user-svc。
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

// GetUser 拉取单个用户的用户名/头像/粉丝数。
// 用于视频详情的作者信息（原实现读 accounts 表，拆分后只能走 gRPC）。
func (c *UserGRPCClient) GetUser(ctx context.Context, userID string) (*pb.UserInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.GetUser(ctx, &pb.GetUserReq{UserId: userID})
}

// GetUsers 批量拉取用户信息，返回 userID → UserInfo 映射。
// 用于公开视频列表、用户视频列表的作者名/头像补全。
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

// IsFollowing 查询 userID 是否关注了 targetUserID（视频详情关注状态回显）。
func (c *UserGRPCClient) IsFollowing(ctx context.Context, userID, targetUserID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	resp, err := c.client.IsFollowing(ctx, &pb.IsFollowingReq{
		UserId:       userID,
		TargetUserId: targetUserID,
	})
	if err != nil {
		return false, err
	}
	return resp.IsFollowing, nil
}

// InteractionGRPCClient 调用 interaction-svc 的 gRPC 客户端。
type InteractionGRPCClient struct {
	client pb.InteractionServiceClient
}

func NewInteractionGRPCClient(addr string) (*InteractionGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, err
	}
	return &InteractionGRPCClient{client: pb.NewInteractionServiceClient(conn)}, nil
}

// GetVideoUserState 拉取当前用户对若干视频的点赞状态。
// 视频详情页用于回显点赞按钮状态；未登录时调用方直接跳过。
func (c *InteractionGRPCClient) GetVideoUserState(ctx context.Context, userID string, videoIDs []uint64) (*pb.VideoUserState, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.GetVideoUserState(ctx, &pb.GetVideoUserStateReq{
		UserId:   userID,
		VideoIds: videoIDs,
	})
}
