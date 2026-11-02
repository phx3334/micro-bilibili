<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { uploadVideo, getVideoStatus } from '@/api/video'
import type { VideoStatusResp } from '@/types'
import { Upload, FileVideo, X, Loader2, CheckCircle, XCircle } from 'lucide-vue-next'

const zones = [
  '番剧', '电影', '国创', '电视剧', '综艺', '纪录片', '动画', '游戏', '鬼畜',
  '音乐', '舞蹈', '影视', '娱乐', '知识', '科技数码', '资讯', '美食', '小剧场',
  '汽车', '时尚美妆', '体育运动', '其他',
]

const videoFile = ref<File | null>(null)
const coverFile = ref<File | null>(null)
const coverPreview = ref('')
const videoTitle = ref('')
const videoDesc = ref('')
const videoZone = ref('动画')
const videoTags = ref('')

const fileInput = ref<HTMLInputElement | null>(null)
const coverInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const uploadProgress = ref(0)
const videoId = ref<number | null>(null)
const status = ref<VideoStatusResp | null>(null)
const pollingTimer = ref<number | null>(null)

const videoValid = computed(
  () => !!videoFile.value && !!videoTitle.value.trim()
)

const statusText = computed(() => {
  const s = status.value?.status
  if (!s) return ''
  const map: Record<string, string> = {
    transcoding: '转码中',
    published: '发布成功',
    failed: '发布失败',
  }
  return map[s] || s
})

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

function handleFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files[0]) {
    videoFile.value = target.files[0]
    videoId.value = null
    status.value = null
    uploadProgress.value = 0
  }
}

function clearFile() {
  videoFile.value = null
  coverFile.value = null
  coverPreview.value = ''
  videoId.value = null
  status.value = null
  uploadProgress.value = 0
  stopPolling()
}

function handleCoverChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files[0]) {
    coverFile.value = target.files[0]
    const reader = new FileReader()
    reader.onload = () => {
      coverPreview.value = reader.result as string
    }
    reader.readAsDataURL(target.files[0])
  }
}

function clearCover() {
  coverFile.value = null
  coverPreview.value = ''
}

async function handleUpload() {
  if (!videoFile.value || !videoTitle.value.trim() || uploading.value) return
  uploading.value = true
  uploadProgress.value = 0
  try {
    const fd = new FormData()
    fd.append('file', videoFile.value)
    fd.append('title', videoTitle.value.trim())
    fd.append('description', videoDesc.value.trim())
    fd.append('zone', videoZone.value)
    if (coverFile.value) {
      fd.append('cover', coverFile.value)
    }
    // 后端 Tags 字段为 []string（form:"tags"），需要逐个 append 而非 JSON 字符串
    videoTags.value
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
      .forEach((tag) => fd.append('tags', tag))
    const res = await uploadVideo(fd, (p) => {
      uploadProgress.value = p
    })
    videoId.value = res.video_id
    startPolling()
  } catch (err: any) {
    // 区分超时错误，给出更友好的提示
    if (err?.code === 'ECONNABORTED') {
      alert('上传超时，请检查网络后重试')
    } else {
      alert(err?.message || '上传失败')
    }
  } finally {
    uploading.value = false
  }
}

function startPolling() {
  stopPolling()
  const tick = async () => {
    if (!videoId.value) return
    try {
      const s = await getVideoStatus(videoId.value)
      status.value = s
      if (s.status === 'published' || s.status === 'failed') {
        stopPolling()
        return
      }
    } catch {
      // 轮询错误忽略，继续重试
    }
    pollingTimer.value = window.setTimeout(tick, 3000)
  }
  tick()
}

function stopPolling() {
  if (pollingTimer.value) {
    clearTimeout(pollingTimer.value)
    pollingTimer.value = null
  }
}

function viewVideo() {
  if (status.value?.video_url) {
    window.open(status.value.video_url, '_blank')
  }
}

onUnmounted(() => {
  stopPolling()
})
</script>

<template>
  <div class="space-y-4">
    <!-- 文件选择 -->
    <div>
      <label class="block text-sm font-medium text-ink mb-2">视频文件</label>
      <div
        v-if="!videoFile"
        class="border-2 border-dashed border-surface-muted rounded-lg p-8 text-center hover:border-primary transition cursor-pointer"
        @click="fileInput?.click()"
      >
        <FileVideo :size="32" class="mx-auto text-ink-muted mb-2" />
        <p class="text-sm text-ink-secondary">点击选择视频文件</p>
        <p class="text-xs text-ink-muted mt-1">支持 MP4 / WebM 等格式</p>
      </div>
      <div v-else class="flex items-center gap-3 bg-surface-subtle rounded-lg p-3">
        <FileVideo :size="20" class="text-secondary shrink-0" />
        <div class="flex-1 min-w-0">
          <div class="text-sm text-ink truncate">{{ videoFile.name }}</div>
          <div class="text-xs text-ink-muted">{{ formatSize(videoFile.size) }}</div>
        </div>
        <button class="text-ink-muted hover:text-primary" @click="clearFile">
          <X :size="18" />
        </button>
      </div>
      <input
        ref="fileInput"
        type="file"
        accept="video/*"
        class="hidden"
        @change="handleFileChange"
      />
    </div>

    <!-- 封面上传 -->
    <div>
      <label class="block text-sm font-medium text-ink mb-2">视频封面（可选）</label>
      <div
        v-if="!coverPreview"
        class="border-2 border-dashed border-surface-muted rounded-lg p-8 text-center hover:border-primary transition cursor-pointer"
        @click="coverInput?.click()"
      >
        <Upload :size="24" class="mx-auto text-ink-muted mb-1" />
        <p class="text-sm text-ink-secondary">点击上传封面图片</p>
        <p class="text-xs text-ink-muted mt-1">支持 JPG / PNG / GIF / WebP</p>
      </div>
      <div v-else class="relative inline-block">
        <img :src="coverPreview" alt="封面预览" class="w-48 h-28 object-cover rounded-lg" />
        <button
          class="absolute -top-2 -right-2 w-6 h-6 bg-black/60 rounded-full text-white flex items-center justify-center text-xs hover:bg-black/80"
          @click="clearCover"
        >
          <X :size="14" />
        </button>
      </div>
      <input
        ref="coverInput"
        type="file"
        accept="image/*"
        class="hidden"
        @change="handleCoverChange"
      />
    </div>

    <!-- 表单字段 -->
    <div>
      <label class="block text-sm font-medium text-ink mb-2">
        标题 <span class="text-primary">*</span>
      </label>
      <input
        v-model="videoTitle"
        type="text"
        maxlength="255"
        placeholder="请输入标题（最长255字符）"
        class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
      />
    </div>

    <div>
      <label class="block text-sm font-medium text-ink mb-2">简介</label>
      <textarea
        v-model="videoDesc"
        rows="3"
        maxlength="512"
        placeholder="可选，最长512字符"
        class="w-full p-4 bg-surface-subtle rounded-lg text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
      />
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <div>
        <label class="block text-sm font-medium text-ink mb-2">分区</label>
        <select
          v-model="videoZone"
          class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
        >
          <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
        </select>
      </div>
      <div>
        <label class="block text-sm font-medium text-ink mb-2">标签</label>
        <input
          v-model="videoTags"
          type="text"
          placeholder="多个标签用英文逗号分隔"
          class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
        />
      </div>
    </div>

    <!-- 上传进度 / 状态 -->
    <div v-if="uploading || videoId" class="bg-surface-subtle rounded-lg p-4">
      <div v-if="uploading">
        <div class="flex justify-between text-sm mb-1">
          <span class="text-ink-secondary">上传中...</span>
          <span class="text-primary">{{ uploadProgress }}%</span>
        </div>
        <div class="h-2 bg-surface-muted rounded-full overflow-hidden">
          <div
            class="h-full bg-primary transition-all duration-200"
            :style="{ width: uploadProgress + '%' }"
          />
        </div>
      </div>
      <div v-else-if="status" class="flex items-center gap-2 text-sm">
        <Loader2
          v-if="status.status === 'transcoding'"
          :size="18"
          class="animate-spin text-secondary"
        />
        <CheckCircle v-else-if="status.status === 'published'" :size="18" class="text-green-500" />
        <XCircle v-else-if="status.status === 'failed'" :size="18" class="text-red-500" />
        <span class="text-ink">{{ statusText }}</span>
        <span v-if="status.fail_reason" class="text-red-500 text-xs">{{ status.fail_reason }}</span>
      </div>
      <div v-else class="text-sm text-ink-muted">等待处理...</div>
    </div>

    <!-- 发布成功 -->
    <div
      v-if="status && status.status === 'published'"
      class="bg-green-50 border border-green-200 rounded-lg p-4 text-sm flex items-center justify-between"
    >
      <span class="text-green-700">视频发布成功！</span>
      <button
        class="px-4 h-9 bg-primary text-white rounded-lg text-sm hover:bg-primary-dark transition"
        @click="viewVideo"
      >
        查看视频
      </button>
    </div>

    <!-- 上传按钮 -->
    <button
      :disabled="!videoValid || uploading"
      class="px-6 h-11 bg-primary text-white rounded-lg text-sm font-medium hover:bg-primary-dark transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
      @click="handleUpload"
    >
      <Upload :size="16" />
      {{ uploading ? '上传中...' : '上传投稿' }}
    </button>
  </div>
</template>
