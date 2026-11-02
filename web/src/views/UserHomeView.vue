<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UserPlus, UserCheck, Pencil, Video as VideoIcon } from 'lucide-vue-next'
import { getUserHome } from '@/api/user'
import { getUserVideos, deleteVideo } from '@/api/video'
import { followUser, unfollowUser } from '@/api/interaction'
import { useUserStore } from '@/stores/user'
import type { HomeVideoInfo, UserHomeResp } from '@/types'
import VideoCard from '@/components/common/VideoCard.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const userId = computed(() => route.params.id as string)

// 用户主页数据
const homeData = ref<UserHomeResp | null>(null)
const isFollowed = ref(false)
const followLoading = ref(false)
const homeLoading = ref(false)

// 视频列表
const videos = ref<HomeVideoInfo[]>([])
const videoPage = ref(1)
const videoTotal = ref(0)
const videoLoading = ref(false)

const isSelf = computed(() => userStore.userInfo?.id === userId.value)

async function loadHomeData() {
  homeLoading.value = true
  try {
    const res = await getUserHome(userId.value, 1, 20)
    homeData.value = res
    // 后端 UserHomeResp.is_followed 已返回"当前登录用户是否关注该用户"，
    // 用它驱动按钮灰态，避免重新加载 / 路由变动后"已关注"状态丢失。
    isFollowed.value = res.is_followed ?? false
    videos.value = res.videos || []
    videoTotal.value = res.video_count || 0
  } catch {
    homeData.value = null
  } finally {
    homeLoading.value = false
  }
}

async function loadVideos(page: number = 1) {
  videoLoading.value = true
  try {
    const res = await getUserVideos(userId.value, page, 20)
    videos.value = res.list || []
    videoTotal.value = res.total || 0
    videoPage.value = page
  } finally {
    videoLoading.value = false
  }
}

// 删除自己投稿的视频（仅 isSelf 时展示删除按钮）
const deletingId = ref<number | null>(null)
async function onDeleteVideo(video: HomeVideoInfo) {
  if (!window.confirm(`确定删除视频《${video.title || '未命名'}》吗？此操作不可恢复。`)) return
  deletingId.value = video.id
  try {
    await deleteVideo(video.id)
    videos.value = videos.value.filter((v) => v.id !== video.id)
    videoTotal.value = Math.max(0, videoTotal.value - 1)
  } catch {
    // 错误提示由请求拦截器统一处理
  } finally {
    deletingId.value = null
  }
}

async function toggleFollow() {
  if (!userStore.isLogin) {
    router.push('/login')
    return
  }
  if (followLoading.value) return
  followLoading.value = true
  try {
    if (isFollowed.value) {
      await unfollowUser(userId.value)
      isFollowed.value = false
    } else {
      await followUser(userId.value)
      isFollowed.value = true
    }
  } finally {
    followLoading.value = false
  }
}

function formatCount(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}

watch(
  () => route.params.id,
  (id) => {
    if (!id) return
    homeData.value = null
    videos.value = []
    loadHomeData()
  },
  { immediate: true }
)
</script>

<template>
  <div class="bili-user-home">
    <!-- 顶部横幅 + 用户信息（B站风格） -->
    <div class="user-banner">
      <!-- 背景横幅 -->
      <div class="h-40 bg-gradient-to-r from-primary/20 via-secondary/20 to-primary/20 relative overflow-hidden">
        <div class="absolute inset-0 bg-gradient-to-b from-transparent to-white/50" />
      </div>

      <!-- 用户信息卡 -->
      <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 -mt-12 relative">
        <div class="bg-white rounded-card shadow-card p-6">
          <div class="flex items-start gap-4">
            <!-- 头像 -->
            <div class="w-20 h-20 rounded-full overflow-hidden border-4 border-white shadow-lg shrink-0 bg-surface-muted">
              <img
                :src="homeData?.avatar_url || '/images/avatar-default.jpg'"
                :alt="homeData?.username"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
              />
            </div>

            <!-- 用户名 + 签名 -->
            <div class="flex-1 min-w-0 pt-2">
              <div class="flex items-center gap-2">
                <h1 class="text-xl font-bold text-ink">{{ homeData?.username || '加载中...' }}</h1>
              </div>
              <p class="text-sm text-ink-muted mt-2">{{ homeData?.signature || '这个人很懒，什么都没留下~' }}</p>
            </div>

            <!-- 操作按钮 -->
            <div class="shrink-0 pt-2">
              <button
                v-if="!isSelf"
                class="flex items-center gap-1 px-5 h-9 rounded-full text-sm font-medium transition disabled:opacity-60"
                :class="isFollowed
                  ? 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
                  : 'bg-primary text-white hover:bg-primary-dark'"
                :disabled="followLoading"
                @click="toggleFollow"
              >
                <component :is="isFollowed ? UserCheck : UserPlus" :size="16" />
                {{ isFollowed ? '已关注' : '关注' }}
              </button>
              <button
                v-else
                class="flex items-center gap-1 px-5 h-9 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
                @click="router.push('/space/profile')"
              >
                <Pencil :size="16" />
                编辑资料
              </button>
            </div>
          </div>

          <!-- 数据统计 -->
          <div class="mt-4 flex items-center gap-6 border-t border-surface-subtle pt-4">
            <div class="text-center cursor-pointer hover:text-primary transition" @click="router.push(`/space/following?user_id=${userId}`)">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.following_count || 0) }}</div>
              <div class="text-xs text-ink-muted">关注</div>
            </div>
            <div class="text-center cursor-pointer hover:text-primary transition" @click="router.push(`/space/followers?user_id=${userId}`)">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.fans_count || 0) }}</div>
              <div class="text-xs text-ink-muted">粉丝</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.total_likes_received || 0) }}</div>
              <div class="text-xs text-ink-muted">获赞</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.total_play_count || 0) }}</div>
              <div class="text-xs text-ink-muted">播放</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab 切换 + 内容区 -->
    <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 mt-4">
      <!-- Tab 栏 -->
      <div class="flex items-center gap-6 mb-4 border-b border-surface-muted">
        <button
          class="relative flex items-center gap-1.5 pb-3 text-sm transition text-primary font-medium"
        >
          <VideoIcon :size="16" />
          投稿
          <span class="text-xs text-ink-muted ml-1">{{ homeData?.video_count || 0 }}</span>
          <span
            class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"
          />
        </button>
      </div>

      <!-- 视频列表 -->
      <div>
        <div v-if="videoLoading && !videos.length" class="text-center py-10 text-sm text-ink-muted">
          加载中...
        </div>
        <div
          v-else-if="videos.length"
          class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4"
        >
          <div v-for="video in videos" :key="video.id" class="relative group">
            <VideoCard :video="video" />
            <button
              v-if="isSelf"
              class="absolute top-2 right-2 z-10 flex items-center gap-1 px-2 h-7 rounded-full text-xs font-medium bg-black/60 text-white opacity-0 group-hover:opacity-100 hover:bg-red-500 transition disabled:opacity-40"
              :disabled="deletingId === video.id"
              @click.stop="onDeleteVideo(video)"
            >
              {{ deletingId === video.id ? '删除中' : '删除' }}
            </button>
          </div>
        </div>
        <EmptyState v-else text="该用户暂无投稿" />
        <Pagination
          v-if="videos.length"
          :current="videoPage"
          :total="videoTotal"
          :page-size="20"
          @change="(p) => loadVideos(p)"
        />
      </div>
    </div>
  </div>
</template>
