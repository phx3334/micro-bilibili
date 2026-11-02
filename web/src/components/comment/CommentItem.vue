<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import { Heart, MessageCircle } from 'lucide-vue-next'
import type { CommentItem } from '@/types'
import { likeComment, unlikeComment } from '@/api/comment'
import { useUserStore } from '@/stores/user'

const props = defineProps<{
  comment: CommentItem
  videoId: number
  targetType?: string
}>()

const emit = defineEmits<{
  (e: 'reply', parentId: number): void
}>()

const userStore = useUserStore()
const liked = ref(false)
const likeCount = ref(props.comment.like_count)
const loading = ref(false)

// localStorage key 按用户 ID 隔离，避免跨用户污染
const LIKED_STORAGE_KEY = computed(() => `fake_doge_liked_comments_${userStore.userInfo?.id || 'guest'}`)

// 从 localStorage 读取已点赞评论 id 集合
function loadLikedSet(): Set<string> {
  try {
    const raw = localStorage.getItem(LIKED_STORAGE_KEY.value)
    if (raw) {
      const arr: string[] = JSON.parse(raw)
      return new Set(arr)
    }
  } catch {
    // ignore
  }
  return new Set()
}

// 写入已点赞评论 id 到 localStorage
function saveLikedSet(set: Set<string>) {
  try {
    localStorage.setItem(LIKED_STORAGE_KEY.value, JSON.stringify(Array.from(set)))
  } catch {
    // ignore
  }
}

const commentKey = computed(() => `${props.targetType || 'video'}:${props.comment.id}`)

onMounted(() => {
  // 从 localStorage 恢复点赞状态
  const likedSet = loadLikedSet()
  liked.value = likedSet.has(commentKey.value)
})

// 当父组件刷新评论列表时，同步服务端返回的最新点赞数
watch(() => props.comment.like_count, (v) => {
  likeCount.value = v
})

async function toggleLike() {
  if (loading.value) return
  loading.value = true
  const wasLiked = liked.value
  // 乐观更新
  liked.value = !wasLiked
  likeCount.value += wasLiked ? -1 : 1
  try {
    if (wasLiked) {
      await unlikeComment(props.targetType || 'video', props.comment.id)
      // 从 localStorage 移除
      const likedSet = loadLikedSet()
      likedSet.delete(commentKey.value)
      saveLikedSet(likedSet)
    } else {
      await likeComment(props.targetType || 'video', props.comment.id)
      // 写入 localStorage
      const likedSet = loadLikedSet()
      likedSet.add(commentKey.value)
      saveLikedSet(likedSet)
    }
  } catch {
    // 回滚
    liked.value = wasLiked
    likeCount.value += wasLiked ? 1 : -1
  } finally {
    loading.value = false
  }
}

function formatTime(time: string): string {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  const diff = (now.getTime() - d.getTime()) / 1000
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  if (diff < 2592000) return `${Math.floor(diff / 86400)}天前`
  return d.toLocaleDateString()
}
</script>

<template>
  <div class="flex gap-3 py-3">
    <div class="relative shrink-0">
      <img
        :src="comment.user?.avatar_url || '/images/avatar-default.jpg'"
        :alt="comment.user?.username"
        class="w-9 h-9 rounded-full object-cover shrink-0 bg-surface-muted"
        @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
      />
    </div>
    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-sm text-ink-secondary font-medium hover:text-primary cursor-pointer">
          {{ comment.user?.username }}
        </span>
      </div>
      <p class="mt-1 text-sm text-ink break-words whitespace-pre-wrap">{{ comment.content }}</p>
      <div class="mt-2 flex items-center gap-4 text-xs text-ink-muted">
        <span>{{ formatTime(comment.created_at) }}</span>
        <button
          class="flex items-center gap-1 hover:text-primary transition disabled:opacity-50"
          :class="{ 'text-primary': liked }"
          :disabled="loading"
          @click="toggleLike"
        >
          <Heart :size="14" :fill="liked ? 'currentColor' : 'none'" />
          <span>{{ likeCount }}</span>
        </button>
        <button
          class="flex items-center gap-1 hover:text-primary transition"
          @click="emit('reply', comment.id)"
        >
          <MessageCircle :size="14" />
          <span>回复</span>
        </button>
      </div>
    </div>
  </div>
</template>
