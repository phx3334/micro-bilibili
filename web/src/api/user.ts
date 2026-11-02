import request from './request'
import type { Account, LoginResp, CaptchaResp, UserHomeResp } from '@/types'

// 用户注册
// 后端 RegisterReq 字段：username / password / captcha_id / captcha（图形验证码）
export function register(payload: { username: string; password: string; captcha_id: string; captcha: string }) {
  return request.post<any, Account>('/user/register', payload)
}

// 用户登录
export function login(username: string, password: string, captcha_id: string, captcha: string) {
  return request.post<any, LoginResp>('/user/login', {
    username,
    password,
    captcha_id,
    captcha,
  })
}

// 退出登录
export function logout() {
  return request.post<any, void>('/user/logout')
}

// 获取个人信息
export function getUserInfo() {
  return request.get<any, Account>('/user/info')
}

// 修改个人信息
// 后端字段：username / signature / avatar / birthday / gender
export function changeUserInfo(payload: {
  username?: string
  signature?: string
  avatar?: string
  birthday?: string
  gender?: string
}) {
  return request.put<any, void>('/user/changeInfo', payload)
}

// 上传头像（multipart/form-data，字段名 avatar）
// 后端返回 { avatar_url: string }
// 注意：不要手动设置 Content-Type，axios 传 FormData 时会自动加上 boundary
export function uploadAvatar(file: File) {
  const fd = new FormData()
  fd.append('avatar', file)
  return request.post<any, { avatar_url: string }>('/user/avatar', fd, {
    timeout: 30000, // 头像上传给 30 秒
  })
}

// 获取图形验证码
export function getCaptcha() {
  return request.post<any, CaptchaResp>('/user/base/captcha')
}

// 获取用户主页信息（含视频列表、统计数据）
export function getUserHome(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, UserHomeResp>('/user/home', { params: { user_id, page, page_size } })
}
