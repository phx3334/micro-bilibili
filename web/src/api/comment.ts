import request from './request'
import type { CommentItem } from '@/types'

// 评论列表（统一接口，通过 target_type 区分目标类型）
export function getCommentList(
  target_type: string,
  target_id: number,
  page: number = 1,
  page_size: number = 20
) {
  return request.get<any, { list: CommentItem[]; total: number }>(
    '/interaction/comment/list',
    { params: { target_type, target_id, page, page_size } }
  )
}

// 回复列表
export function getCommentReplies(
  target_type: string,
  comment_id: number,
  page: number = 1,
  page_size: number = 20
) {
  return request.get<any, { list: CommentItem[]; total: number }>(
    '/interaction/comment/replies',
    { params: { target_type, comment_id, page, page_size } }
  )
}

// 创建评论
export function createComment(
  target_type: string,
  target_id: number,
  content: string,
  parent_id: number = 0
) {
  return request.post<any, { comment_id: number }>('/interaction/comment/create', {
    target_type,
    target_id,
    parent_id,
    content,
  })
}

// 点赞评论
export function likeComment(target_type: string, comment_id: number) {
  return request.post<any, void>('/interaction/comment/like', { target_type, comment_id })
}

// 取消点赞评论
export function unlikeComment(target_type: string, comment_id: number) {
  return request.post<any, void>('/interaction/comment/unlike', { target_type, comment_id })
}

// 删除评论
export function deleteComment(target_type: string, comment_id: number) {
  return request.post<any, void>('/interaction/comment/delete', { target_type, comment_id })
}
