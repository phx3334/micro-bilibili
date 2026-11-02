package user

import (
	"context"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/storagex"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UserGRPCServer user-svc 的 gRPC 服务实现。
// 供 video/interaction 服务批量拉取用户名、头像等展示数据。
// DB 中头像保存对象 key，gRPC 响应在序列化时转成可访问 URL。
type UserGRPCServer struct {
	pb.UnimplementedUserServiceServer
	accountRepo *AccountRepo
	followRepo  *FollowRepo
	storage     storagex.Storage
}

func NewUserGRPCServer(accountRepo *AccountRepo, followRepo *FollowRepo, storage storagex.Storage) *UserGRPCServer {
	return &UserGRPCServer{accountRepo: accountRepo, followRepo: followRepo, storage: storage}
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *pb.GetUserReq) (*pb.UserInfo, error) {
	account, err := s.accountRepo.FindByID(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return s.toUserInfo(ctx, account), nil
}

func (s *UserGRPCServer) GetUsers(ctx context.Context, req *pb.GetUsersReq) (*pb.GetUsersResp, error) {
	if len(req.UserIds) == 0 {
		return &pb.GetUsersResp{Users: []*pb.UserInfo{}}, nil
	}
	accounts, err := s.accountRepo.FindByIDs(ctx, req.UserIds)
	if err != nil {
		return nil, status.Error(codes.Internal, "query users failed")
	}
	resp := &pb.GetUsersResp{Users: make([]*pb.UserInfo, 0, len(accounts))}
	for _, a := range accounts {
		resp.Users = append(resp.Users, s.toUserInfo(ctx, a))
	}
	return resp, nil
}

// IsFollowing 查询 user_id 是否关注了 target_user_id。
// 未登录（空 user_id）、查看自己或目标为空时直接返回 false，避免无意义查询。
func (s *UserGRPCServer) IsFollowing(ctx context.Context, req *pb.IsFollowingReq) (*pb.IsFollowingResp, error) {
	if req.UserId == "" || req.TargetUserId == "" || req.UserId == req.TargetUserId {
		return &pb.IsFollowingResp{}, nil
	}
	followed, err := s.followRepo.IsUserFollowed(ctx, req.UserId, req.TargetUserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "query follow relation failed")
	}
	return &pb.IsFollowingResp{IsFollowing: followed}, nil
}

func (s *UserGRPCServer) toUserInfo(ctx context.Context, a *model.Account) *pb.UserInfo {
	avatarURL, _ := s.storage.URL(ctx, a.AvatarURL)
	info := &pb.UserInfo{
		Id:        a.ID,
		Username:  a.Username,
		AvatarUrl: avatarURL,
		Signature: a.Signature,
		Gender:    a.Gender,
		Birthday:  a.Birthday,
	}
	// 粉丝/关注数在 user_follows 表内，随账户一起返回给下游展示
	if n, err := s.followRepo.GetFansCount(ctx, a.ID); err == nil {
		info.FollowerCount = n
	}
	if n, err := s.followRepo.GetFollowingCount(ctx, a.ID); err == nil {
		info.FollowingCount = n
	}
	return info
}
