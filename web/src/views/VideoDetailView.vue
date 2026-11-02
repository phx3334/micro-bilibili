<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Heart, Share2, Bell, Play } from 'lucide-vue-next'
import { getVideoDetail, getHomeVideoList } from '@/api/video'
import {
  likeVideo,
  unlikeVideo,
  followUser,
  unfollowUser,
} from '@/api/interaction'
import { useUserStore } from '@/stores/user'
import type { VideoDetailResp, HomeVideoInfo } from '@/types'
import VideoPlayer from '@/components/video/VideoPlayer.vue'
import CommentTree from '@/components/comment/CommentTree.vue'
import VideoCard from '@/components/common/VideoCard.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const videoId = computed(() => Number(route.params.id))
const video = ref<VideoDetailResp | null>(null)
const recommendList = ref<HomeVideoInfo[]>([])
const loading = ref(true)

// 交互状态
const isLiked = ref(false)
const isFollowed = ref(false)

const playCountText = computed(() => {
  const n = video.value?.play_count || 0
  return n >= 10000 ? (n / 10000).toFixed(1) + '万' : String(n)
})

function showToast(msg: string) {
  const toast = document.createElement('div')
  toast.textContent = msg
  toast.style.cssText =
    'position:fixed;top:60px;left:50%;transform:translateX(-50%);background:#333;color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px;box-shadow:0 2px 8px rgba(0,0,0,0.15)'
  document.body.appendChild(toast)
  setTimeout(() => toast.remove(), 2500)
}

function requireLogin(): boolean {
  if (!userStore.isLogin) {
    showToast('请先登录')
    router.push('/login')
    return false
  }
  return true
}

async function loadVideoDetail() {
  loading.value = true
  try {
    const res = await getVideoDetail(videoId.value)
    video.value = res
    isLiked.value = res.interaction.is_liked
    isFollowed.value = res.interaction.is_followed
  } catch {
    video.value = null
  } finally {
    loading.value = false
  }
}

async function loadRecommend() {
  try {
    const res = await getHomeVideoList('', 6)
    recommendList.value = (res.list || []).filter((v) => v.id !== videoId.value).slice(0, 5)
  } catch {
    recommendList.value = []
  }
}

async function toggleLike() {
  if (!requireLogin() || !video.value) return
  const wasLiked = isLiked.value
  isLiked.value = !wasLiked
  video.value.likes_count += wasLiked ? -1 : 1
  try {
    if (wasLiked) {
      await unlikeVideo(videoId.value)
    } else {
      await likeVideo(videoId.value)
    }
  } catch {
    isLiked.value = wasLiked
    video.value.likes_count += wasLiked ? 1 : -1
  }
}

async function toggleFollow() {
  if (!requireLogin() || !video.value) return
  const wasFollowed = isFollowed.value
  isFollowed.value = !wasFollowed
  // 乐观更新粉丝数
  video.value.author.fans_count += wasFollowed ? -1 : 1
  try {
    if (wasFollowed) {
      await unfollowUser(video.value.author.id)
      showToast('已取消关注')
    } else {
      await followUser(video.value.author.id)
      showToast('关注成功')
    }
  } catch {
    isFollowed.value = wasFollowed
    video.value.author.fans_count += wasFollowed ? 1 : -1
  }
}

async function initPage() {
  // 先加载视频详情
  await loadVideoDetail()
  loadRecommend()
}

onMounted(() => {
  initPage()
})

// 路由参数变化时重新加载
watch(() => route.params.id, async (newId) => {
  if (newId) {
    video.value = null
    loading.value = true
    // 重置交互状态，避免新视频继承旧视频的点赞/关注状态
    isLiked.value = false
    isFollowed.value = false
    await loadVideoDetail()
    loadRecommend()
  }
})
</script>

<template>
  <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 py-4">
    <!-- 骨架屏 -->
    <div v-if="loading" class="flex gap-4">
      <div class="flex-1">
        <div class="w-full aspect-video bg-surface-muted animate-pulse rounded" />
        <div class="h-7 bg-surface-muted rounded animate-pulse mt-4 w-3/4" />
        <div class="h-12 bg-surface-muted rounded animate-pulse mt-3" />
      </div>
      <div class="w-[320px] space-y-3">
        <div v-for="i in 5" :key="i" class="h-20 bg-surface-muted rounded animate-pulse" />
      </div>
    </div>

    <div v-else-if="video" class="flex gap-4">
      <!-- 左侧主区域 -->
      <div class="flex-1 min-w-0">
        <!-- 标题（B站风格：标题在播放器上方） -->
        <h1 class="mb-3 text-xl font-medium text-ink leading-snug">{{ video.title }}</h1>

        <!-- 播放器 -->
        <VideoPlayer
          :url="video.play_url"
          :cover-url="video.cover_url"
        />

        <!-- 数据统计栏 -->
        <div class="mt-3 flex items-center gap-4 text-xs text-ink-muted px-1">
          <span class="flex items-center gap-1">
            <Play :size="14" />
            {{ playCountText }} 播放
          </span>
          <span class="flex items-center gap-1">
            <Heart :size="14" />
            {{ video.likes_count }} 点赞
          </span>
          <span v-if="video.zone" class="ml-auto px-2 py-0.5 bg-surface-subtle rounded text-ink-secondary">
            {{ video.zone }}
          </span>
        </div>

        <!-- UP主信息卡 + 互动栏（B站风格：合并为一行） -->
        <div class="mt-3 bg-white rounded-card p-4">
          <div class="flex items-center gap-3">
            <!-- UP主头像 -->
            <img
              :src="video.author.avatar_url || '/images/avatar-default.jpg'"
              :alt="video.author.username"
              class="w-12 h-12 rounded-full object-cover bg-surface-muted shrink-0 cursor-pointer border-2 border-surface-muted"
              @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
              @click="router.push(`/user/${video.author.id}`)"
            />
            <div class="flex-1 min-w-0 cursor-pointer" @click="router.push(`/user/${video.author.id}`)">
              <div class="text-sm font-medium text-ink truncate hover:text-primary transition">{{ video.author.username }}</div>
              <div class="text-xs text-ink-muted">{{ video.author.fans_count }} 粉丝</div>
            </div>
            <!-- 关注按钮 -->
            <button
              class="flex items-center gap-1 px-4 h-8 rounded text-sm font-medium transition shrink-0"
              :class="
                isFollowed
                  ? 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
                  : 'bg-primary text-white hover:bg-primary-dark'
              "
              @click="toggleFollow"
            >
              <Bell :size="14" />
              {{ isFollowed ? '已关注' : '关注' }}
            </button>
          </div>

          <!-- 分隔线 -->
          <div class="my-3 border-t border-surface-subtle" />

          <!-- 互动工具栏（B站风格：圆角按钮） -->
          <div class="flex items-center gap-2 flex-wrap">
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium transition"
              :class="
                isLiked
                  ? 'bg-primary/10 text-primary'
                  : 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
              "
              @click="toggleLike"
            >
              <Heart :size="18" :fill="isLiked ? 'currentColor' : 'none'" />
              <span>{{ video.likes_count }}</span>
            </button>
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
            >
              <Share2 :size="18" />
              <span>分享</span>
            </button>
          </div>
        </div>

        <!-- 视频简介 -->
        <div v-if="video.description" class="mt-3 p-4 bg-white rounded-card">
          <h3 class="text-sm font-medium text-ink mb-2">视频简介</h3>
          <p class="text-sm text-ink-secondary whitespace-pre-wrap break-words leading-relaxed">
            {{ video.description }}
          </p>
        </div>

        <!-- 评论区 -->
        <div class="mt-4 p-4 bg-white rounded-card">
          <h3 class="text-base font-medium text-ink mb-4 flex items-center gap-2">
            <span>评论</span>
            <span class="text-sm text-ink-muted">{{ video.comment_count }}</span>
          </h3>
          <CommentTree :video-id="videoId" comment-type="video" />
        </div>
      </div>

      <!-- 右侧推荐 -->
      <div class="w-[320px] shrink-0 space-y-3 hidden lg:block">
        <h3 class="text-sm font-medium text-ink flex items-center gap-2">
          <span class="w-1 h-4 bg-secondary rounded-full" />
          推荐视频
        </h3>
        <VideoCard v-for="rec in recommendList" :key="rec.id" :video="rec" />
        <EmptyState v-if="!recommendList.length" text="暂无推荐" />
      </div>
    </div>

    <EmptyState v-else text="视频不存在或已下架" />
  </div>
</template>
