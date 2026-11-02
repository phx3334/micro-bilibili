<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { HomeVideoInfo } from '@/types'

const props = defineProps<{
  video: HomeVideoInfo
}>()

const router = useRouter()

const formattedPlayCount = computed(() => {
  const n = props.video.play_count
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
})

const formattedCommentCount = computed(() => {
  const n = props.video.comment_count
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
})

const formattedDuration = computed(() => {
  const d = props.video.duration
  const min = Math.floor(d / 60)
  const sec = Math.floor(d % 60)
  return `${min}:${String(sec).padStart(2, '0')}`
})

function goVideo() {
  router.push(`/video/${props.video.id}`)
}

function goUser(e: MouseEvent) {
  e.stopPropagation()
  // 跳转到用户主页 - 通过 up_name 无法直接跳转，需要 user_id
  // VideoCard 中无 user_id 字段，暂不跳转
}
</script>

<template>
  <div
    class="bili-video-card group cursor-pointer"
    @click="goVideo"
  >
    <!-- 封面 -->
    <div class="bili-video-card__cover relative aspect-video bg-surface-muted overflow-hidden rounded">
      <img
        :src="video.cover_url"
        :alt="video.title"
        loading="lazy"
        class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
      />
      <!-- 悬停遮罩 -->
      <div class="absolute inset-0 bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
      <!-- 时长 -->
      <span class="absolute bottom-1 right-1 px-1 py-0.5 bg-black/70 text-white text-xs rounded leading-tight">
        {{ formattedDuration }}
      </span>
      <!-- 悬停播放量/弹幕 -->
      <div class="absolute bottom-0 left-0 right-0 h-7 bg-gradient-to-t from-black/70 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center px-2 gap-3 text-white text-xs">
        <span class="flex items-center gap-0.5">
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
          {{ formattedPlayCount }}
        </span>
        <span class="flex items-center gap-0.5">
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h14l4 4V4c0-1.1-.9-2-2-2z"/></svg>
          {{ formattedCommentCount }}
        </span>
      </div>
    </div>
    <!-- 信息 -->
    <div class="mt-2">
      <h3 class="text-sm text-ink leading-5 text-ellipsis-2 min-h-[40px] group-hover:text-secondary transition-colors">
        {{ video.title }}
      </h3>
      <div class="mt-1.5 flex items-center gap-1.5 text-xs text-ink-muted">
        <img
          :src="video.up_avatar || '/images/avatar-default.jpg'"
          :alt="video.up_name"
          class="w-4 h-4 rounded-full object-cover shrink-0"
          @error="($event.target as HTMLImageElement).style.display = 'none'"
        />
        <span class="truncate">{{ video.up_name }}</span>
      </div>
      <div class="mt-1 flex items-center gap-3 text-xs text-ink-muted">
        <span>播放 {{ formattedPlayCount }}</span>
      </div>
    </div>
  </div>
</template>
