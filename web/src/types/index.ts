// 统一响应类型
export interface ApiResponse<T = any> {
  code: 3 | 4 // 3=成功, 4=失败
  data: T
  msg: string
}

// 分页响应
export interface PaginatedResp<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

// 用户
export interface Account {
  id: string
  username: string
  avatar_url: string
  signature: string
  // 编辑资料页使用（后端 /user/info 返回的扩展字段）
  gender?: 'male' | 'female' | 'secret' | string
  birthday?: string
}

export interface LoginResp {
  access_token: string
  access_token_expire: number
  account: Account
}
// 视频
export interface HomeVideoInfo {
  id: number
  up_name: string
  up_avatar: string
  title: string
  cover_url: string
  play_count: number
  comment_count: number
  duration: number
  created_at: string
}

export interface AuthorInfo {
  id: string
  username: string
  avatar_url: string
  signature: string
  fans_count: number
}

export interface InteractionStatusResp {
  is_liked: boolean
  is_followed: boolean
}

export interface VideoDetailResp {
  id: number
  title: string
  description: string
  play_url: string
  cover_url: string
  duration: number
  zone: string
  play_count: number
  likes_count: number
  comment_count: number
  created_at: string
  author: AuthorInfo
  interaction: InteractionStatusResp
}

export interface VideoStatusResp {
  status: 'transcoding' | 'published' | 'failed'
  fail_reason: string
  video_url: string
  cover_url: string
}

// 评论用户卡片
export interface CommentUserCard {
  id: string
  username: string
  avatar_url: string
}

// 评论（统一结构）
export interface CommentItem {
  id: number
  user: CommentUserCard
  content: string
  like_count: number
  created_at: string
  reply_count: number
  replies: CommentItem[]
}

// 关注
export interface FollowUserItem {
  id: string
  username: string
  avatar_url: string
  signature: string
  is_followed?: boolean
}

// 用户主页
export interface UserHomeResp {
  id: string
  username: string
  avatar_url: string
  signature: string
  video_count: number
  birthday: string
  gender: string
  total_likes_received: number
  total_play_count: number
  fans_count: number
  following_count: number
  is_followed?: boolean
  videos: HomeVideoInfo[]
}

// 验证码
export interface CaptchaResp {
  captcha_id: string
  pic_path: string
}
