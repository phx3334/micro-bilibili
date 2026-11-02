import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue'),
  },
  {
    path: '/video/:id',
    name: 'video',
    component: () => import('@/views/VideoDetailView.vue'),
  },
  {
    path: '/user/:id',
    name: 'user',
    component: () => import('@/views/UserHomeView.vue'),
  },
  {
    path: '/space',
    component: () => import('@/views/SpaceView.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/space/profile' },
      {
        path: 'profile',
        name: 'space-profile',
        component: () => import('@/views/space/ProfileEditView.vue'),
      },
      {
        path: 'followers',
        name: 'space-followers',
        component: () => import('@/views/space/FollowListView.vue'),
        props: { mode: 'followers' },
      },
      {
        path: 'following',
        name: 'space-following',
        component: () => import('@/views/space/FollowListView.vue'),
        props: { mode: 'following' },
      },
    ],
  },
  {
    path: '/upload',
    name: 'upload',
    component: () => import('@/views/UploadView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

// 路由守卫
router.beforeEach((to, _from, next) => {
  const userStore = useUserStore()
  if (to.meta.requiresAuth && !userStore.isLogin) {
    next({ path: '/login', query: { redirect: to.fullPath } })
  } else {
    next()
  }
})

router.afterEach((to) => {
  const titles: Record<string, string> = {
    home: '首页',
    video: '视频',
    user: '用户主页',
    'space-profile': '个人资料',
    'space-followers': '我的粉丝',
    'space-following': '我的关注',
    upload: '投稿',
    login: '登录',
  }
  document.title = `${titles[to.name as string] || ''} - 仿B站`
})

export default router
