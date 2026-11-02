import request from './request'
import type { HomeVideoInfo, VideoDetailResp, VideoStatusResp, PaginatedResp } from '@/types'

// 首页视频列表（游标分页）
export function getHomeVideoList(cursor: string = '', limit: number = 20, zone: string = '') {
  return request.get<any, { list: HomeVideoInfo[]; next_cursor: string; has_more: boolean }>(
    '/video/list',
    { params: { cursor, limit, zone } }
  )
}

// 视频详情
export function getVideoDetail(video_id: number) {
  return request.get<any, VideoDetailResp>('/video/detail', { params: { video_id } })
}

// 视频上传
// 注意：不要手动设置 Content-Type，axios 传 FormData 时会自动加上 boundary
// 返回 { video_id: number }，前端据此轮询转码状态
// 超时设置为 1 小时（3600000ms）：大文件上传需要足够时间，超时后取消避免无限等待
export function uploadVideo(
  formData: FormData,
  onProgress?: (percent: number) => void
) {
  return request.post<any, { video_id: number }>('/video/upload', formData, {
    timeout: 3600000, // 1 小时超时，超时后 axios 自动取消请求（支持大文件上传）
    onUploadProgress: (e) => {
      if (onProgress && e.total) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    },
  })
}

// 查询转码状态
export function getVideoStatus(video_id: number) {
  return request.get<any, VideoStatusResp>('/video/status', { params: { video_id } })
}

// 用户主页视频列表
export function getUserVideos(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<HomeVideoInfo>>('/user/videos', {
    params: { user_id, page, page_size },
  })
}

// 删除自己投稿的视频（作者权限由后端校验）
// 注意：后端用 ShouldBindQuery 读取 video_id，故必须放 params（query 字符串），
// 不能放 data（请求 body），否则后端绑定失败报"参数错误"。
export function deleteVideo(video_id: number) {
  return request.delete('/video', { params: { video_id } })
}
