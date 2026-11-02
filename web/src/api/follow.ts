import request from './request'
import type { FollowUserItem, PaginatedResp } from '@/types'

// 粉丝列表
export function getFollowers(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<FollowUserItem>>('/user/follow/followers', {
    params: { user_id, page, page_size },
  })
}

// 关注列表
export function getFollowing(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<FollowUserItem>>('/user/follow/following', {
    params: { user_id, page, page_size },
  })
}
