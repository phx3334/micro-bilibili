<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { PenSquare, User, LogOut, ChevronDown, Home } from 'lucide-vue-next'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const showUserMenu = ref(false)
const avatarError = ref(false)

const avatarUrl = computed(() => {
  if (avatarError.value) return '/images/avatar-default.jpg'
  return userStore.userInfo?.avatar_url || '/images/avatar-default.jpg'
})

// 顶部分区导航（B站风格：推荐 + 22 个分区，分两行，每行 11 个）
const categoryTabs = [
  { key: '番剧', label: '番剧' },
  { key: '电影', label: '电影' },
  { key: '国创', label: '国创' },
  { key: '电视剧', label: '电视剧' },
  { key: '综艺', label: '综艺' },
  { key: '纪录片', label: '纪录片' },
  { key: '动画', label: '动画' },
  { key: '游戏', label: '游戏' },
  { key: '鬼畜', label: '鬼畜' },
  { key: '音乐', label: '音乐' },
  { key: '舞蹈', label: '舞蹈' },
  { key: '影视', label: '影视' },
  { key: '娱乐', label: '娱乐' },
  { key: '知识', label: '知识' },
  { key: '科技数码', label: '科技数码' },
  { key: '资讯', label: '资讯' },
  { key: '美食', label: '美食' },
  { key: '小剧场', label: '小剧场' },
  { key: '汽车', label: '汽车' },
  { key: '时尚美妆', label: '时尚美妆' },
  { key: '体育运动', label: '体育运动' },
  { key: '其他', label: '其他' },
]

const activeZone = computed(() => (route.name === 'home' ? (route.query.zone as string) || '' : ''))

function switchZone(zoneKey: string) {
  if (zoneKey === '') {
    router.push({ path: '/', query: {} })
  } else {
    router.push({ path: '/', query: { zone: zoneKey } })
  }
}

function goLogin() {
  router.push('/login')
}

function goSpace() {
  showUserMenu.value = false
  const id = userStore.userInfo?.id
  if (id) router.push(`/user/${id}`)
  else router.push('/space/profile')
}

async function handleLogout() {
  showUserMenu.value = false
  await userStore.logout()
  router.push('/')
}

function toggleUserMenu() {
  showUserMenu.value = !showUserMenu.value
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.user-menu-wrapper')) {
    showUserMenu.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
/* 首页：导航覆盖在横幅图上，logo 与主导航文字改为白色 */
.home-header .flex.items-center > a span,
.home-header nav a {
  color: #fff !important;
}
.home-header nav a.border-primary {
  border-color: #fff !important;
}
.home-header nav a:not(.border-primary):hover {
  color: #fff !important;
}
</style>

<template>
  <header
    class="bili-header sticky top-0 z-50 min-[1440px]:max-w-none"
    :class="route.name === 'home' ? 'home-header bg-white' : 'bg-white border-b border-surface-subtle'"
  >
    <!-- 首页横幅：纯渐变背景，不依赖外部图片资源（导航覆盖其上，无顶部留白） -->
    <div v-if="route.name === 'home'" class="relative">
      <div class="w-full aspect-[6/1] bg-gradient-to-r from-[#fb7299]/15 via-white to-[#00a1d6]/10 [mask-image:linear-gradient(to_bottom,black_40%,transparent)]"></div>
      <!-- 导航浮于图片之上 -->
      <div class="absolute inset-x-0 top-0 h-14 flex items-center gap-4 px-4 max-w-[1280px] mx-auto min-[1440px]:max-w-none">
        <!-- Logo -->
        <router-link to="/" class="flex items-center gap-1 shrink-0">
          <span class="text-primary text-2xl font-bold tracking-tight">doge</span>
          <span class="text-[10px] leading-none mt-1 text-white">doge</span>
        </router-link>

        <!-- 主导航 -->
        <nav class="flex items-center gap-1 text-sm shrink-0">
          <router-link
            to="/"
            class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
            :class="route.name === 'home' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
          >
            <Home :size="16" />
            <span>首页</span>
          </router-link>
        </nav>

        <div class="flex-1" />

        <!-- 右侧操作 -->
        <div class="flex items-center gap-3 shrink-0">
          <!-- 投稿按钮 -->
          <router-link
            to="/upload"
            class="flex items-center gap-1 px-3 h-8 rounded border border-primary text-primary text-sm hover:bg-primary hover:text-white transition"
          >
            <PenSquare :size="16" />
            <span class="hidden sm:inline">投稿</span>
          </router-link>

          <!-- 头像 -->
          <div class="relative user-menu-wrapper">
            <button
              @click="userStore.isLogin ? toggleUserMenu() : goLogin()"
              class="flex items-center gap-1 h-8 rounded-full hover:bg-surface-subtle pr-1 transition"
              :class="{ 'pl-1': userStore.isLogin }"
            >
              <div class="w-7 h-7 rounded-full overflow-hidden border border-surface-muted">
                <img
                  :src="avatarUrl"
                  :alt="userStore.isLogin ? (userStore.userInfo?.username || 'avatar') : 'login'"
                  class="w-full h-full object-cover"
                  @error="avatarError = true"
                />
              </div>
              <ChevronDown v-if="userStore.isLogin" :size="14" class="text-ink-muted" />
            </button>

            <div
              v-if="userStore.isLogin && showUserMenu"
              class="absolute right-0 top-full mt-1 w-48 bg-white rounded-lg shadow-lg border border-surface-subtle py-1 z-50 animate-fade-in"
            >
              <div class="px-3 py-2 border-b border-surface-subtle">
                <div class="text-sm font-medium text-ink truncate">{{ userStore.userInfo?.username || '用户' }}</div>
              </div>
              <button
                class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
                @click="goSpace"
              >
                <User :size="16" />
                <span>个人中心</span>
              </button>
              <button
                class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
                @click="handleLogout"
              >
                <LogOut :size="16" />
                <span>退出登录</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 分区导航子栏（22 个分区，分两行每行 11 个，仅首页，随图片一起吸顶） -->
      <div class="border-t border-surface-subtle bg-white">
        <div class="max-w-[1280px] mx-auto px-4 py-2 min-[1440px]:max-w-none">
          <div class="grid grid-cols-11 gap-x-2 gap-y-1">
            <button
              v-for="zone in categoryTabs"
              :key="zone.key"
              class="px-3 h-8 rounded-full text-sm whitespace-nowrap text-center transition-colors"
              :class="activeZone === zone.key ? 'bg-primary text-white font-medium' : 'text-ink-secondary hover:bg-surface-subtle hover:text-ink'"
              @click="switchZone(zone.key)"
            >
              {{ zone.label }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 非首页：正常 sticky 顶栏 -->
    <div v-else class="max-w-[1280px] mx-auto h-14 px-4 flex items-center gap-4 min-[1440px]:max-w-none">
      <!-- Logo -->
      <router-link to="/" class="flex items-center gap-1 shrink-0">
        <span class="text-primary text-2xl font-bold tracking-tight">doge</span>
        <span class="text-[10px] leading-none mt-1 text-white">doge</span>
      </router-link>

      <!-- 主导航 -->
      <nav class="flex items-center gap-1 text-sm shrink-0">
        <router-link
          to="/"
          class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
          :class="route.name === 'home' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
        >
          <Home :size="16" />
          <span>首页</span>
        </router-link>
      </nav>

      <div class="flex-1" />

      <!-- 右侧操作 -->
      <div class="flex items-center gap-3 shrink-0">
        <!-- 投稿按钮 -->
        <router-link
          to="/upload"
          class="flex items-center gap-1 px-3 h-8 rounded border border-primary text-primary text-sm hover:bg-primary hover:text-white transition"
        >
          <PenSquare :size="16" />
          <span class="hidden sm:inline">投稿</span>
        </router-link>

        <!-- 头像：登录显示用户头像并展开菜单；未登录显示默认头像并跳转登录 -->
        <div class="relative user-menu-wrapper">
          <button
            @click="userStore.isLogin ? toggleUserMenu() : goLogin()"
            class="flex items-center gap-1 h-8 rounded-full hover:bg-surface-subtle pr-1 transition"
            :class="{ 'pl-1': userStore.isLogin }"
          >
            <div class="w-7 h-7 rounded-full overflow-hidden border border-surface-muted">
              <img
                :src="avatarUrl"
                :alt="userStore.isLogin ? (userStore.userInfo?.username || 'avatar') : 'login'"
                class="w-full h-full object-cover"
                @error="avatarError = true"
              />
            </div>
            <ChevronDown v-if="userStore.isLogin" :size="14" class="text-ink-muted" />
          </button>

          <!-- 已登录时显示下拉菜单 -->
          <div
            v-if="userStore.isLogin && showUserMenu"
            class="absolute right-0 top-full mt-1 w-48 bg-white rounded-lg shadow-lg border border-surface-subtle py-1 z-50 animate-fade-in"
          >
            <div class="px-3 py-2 border-b border-surface-subtle">
              <div class="text-sm font-medium text-ink truncate">{{ userStore.userInfo?.username || '用户' }}</div>
            </div>
            <button
              class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
              @click="goSpace"
            >
              <User :size="16" />
              <span>个人中心</span>
            </button>
            <button
              class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
              @click="handleLogout"
            >
              <LogOut :size="16" />
              <span>退出登录</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>
