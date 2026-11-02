<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import Artplayer from 'artplayer'

const props = defineProps<{
  url: string
  coverUrl: string
}>()

const container = ref<HTMLDivElement | null>(null)
const errorMsg = ref('')
let art: Artplayer | null = null

onMounted(() => {
  if (!container.value) return
  if (!props.url) {
    errorMsg.value = '视频地址无效，可能仍在转码中，请稍后刷新重试'
    return
  }
  art = new Artplayer({
    container: container.value,
    url: props.url,
    poster: props.coverUrl,
    volume: 0.7,
    autoplay: false,
    pip: true,
    setting: true,
    playbackRate: true,
    fullscreen: true,
    fullscreenWeb: true,
  })

  art.on('error', () => {
    errorMsg.value = '视频加载失败，可能仍在转码中，请稍后刷新重试'
  })
})

onUnmounted(() => {
  if (art) {
    art.destroy(false)
    art = null
  }
})
</script>

<template>
  <div>
    <div class="w-full aspect-video bg-black rounded-card overflow-hidden relative">
      <div ref="container" class="w-full aspect-video" />
      <!-- 错误提示 -->
      <div
        v-if="errorMsg"
        class="absolute inset-0 flex items-center justify-center text-white/80 text-sm bg-black/90"
      >
        {{ errorMsg }}
      </div>
    </div>
  </div>
</template>
