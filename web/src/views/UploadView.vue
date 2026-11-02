<script setup lang="ts">
import { useUserStore } from '@/stores/user'
import VideoUpload from '@/views/upload/VideoUpload.vue'

const userStore = useUserStore()
</script>

<template>
  <div class="max-w-[860px] mx-auto px-4 py-6">
    <h1 class="text-2xl font-bold text-ink mb-1">视频投稿</h1>
    <p v-if="userStore.userInfo" class="text-sm text-ink-muted mb-4">
      Hi, {{ userStore.userInfo.username }}，开始你的创作吧
    </p>

    <VideoUpload />
  </div>
</template>
