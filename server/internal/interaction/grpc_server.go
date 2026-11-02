package interaction

import (
	"context"

	"fake_tiktok/api/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InteractionGRPCServer interaction-svc 的 gRPC 服务实现。
// 目前只提供视频点赞状态批量查询，供 video-svc 详情页回显点赞按钮状态。
type InteractionGRPCServer struct {
	pb.UnimplementedInteractionServiceServer
	interactionRepo *InteractionRepo
}

func NewInteractionGRPCServer(interactionRepo *InteractionRepo) *InteractionGRPCServer {
	return &InteractionGRPCServer{interactionRepo: interactionRepo}
}

func (s *InteractionGRPCServer) GetVideoUserState(ctx context.Context, req *pb.GetVideoUserStateReq) (*pb.VideoUserState, error) {
	if req.UserId == "" || len(req.VideoIds) == 0 {
		return &pb.VideoUserState{
			LikedVideoIds: []uint64{},
		}, nil
	}

	liked, err := s.interactionRepo.ListLikedVideoIDs(ctx, req.UserId, req.VideoIds)
	if err != nil {
		return nil, status.Error(codes.Internal, "query like state failed")
	}

	return &pb.VideoUserState{
		LikedVideoIds: liked,
	}, nil
}
