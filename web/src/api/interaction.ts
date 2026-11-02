import request from './request'

// 点赞视频
export function likeVideo(video_id: number) {
  return request.post<any, void>('/interaction/video/like', { video_id })
}

// 取消点赞
export function unlikeVideo(video_id: number) {
  return request.post<any, void>('/interaction/video/unlike', { video_id })
}

// 关注用户（已迁至 user-svc）
// 后端 FollowUserReq 字段为 target_user_id（json:"target_user_id"）
export function followUser(target_user_id: string) {
  return request.post<any, void>('/user/follow', { target_user_id })
}

// 取消关注（已迁至 user-svc）
export function unfollowUser(target_user_id: string) {
  return request.post<any, void>('/user/unfollow', { target_user_id })
}
