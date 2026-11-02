<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Camera, Loader2 } from 'lucide-vue-next'
import { getUserInfo, changeUserInfo, uploadAvatar } from '@/api/user'
import { useUserStore } from '@/stores/user'
import type { Account } from '@/types'

const userStore = useUserStore()

const form = ref({
  username: '',
  signature: '',
  gender: 'secret' as 'male' | 'female' | 'secret',
  birthday: '',
})
const avatarUrl = ref('')
const saving = ref(false)
const uploadingAvatar = ref(false)

// 简单 toast
function showToast(msg: string, ok = true) {
  if (typeof window === 'undefined') return
  const t = document.createElement('div')
  t.textContent = msg
  t.style.cssText = `position:fixed;top:20px;left:50%;transform:translateX(-50%);background:${ok ? 'rgba(0,0,0,.8)' : '#ff4d4f'};color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px`
  document.body.appendChild(t)
  setTimeout(() => t.remove(), 2500)
}

async function loadProfile() {
  try {
    const data: any = await getUserInfo()
    applyAccount(data)
  } catch {
    // 拉取失败时回退到 store 里的
    if (userStore.userInfo) applyAccount(userStore.userInfo)
  }
}

function applyAccount(a: Account | any) {
  form.value.username = a.username || ''
  form.value.signature = a.signature || ''
  form.value.gender = (a.gender as any) || 'secret'
  form.value.birthday = a.birthday || ''
  avatarUrl.value = a.avatar_url || ''
}

async function handleAvatarChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    showToast('请选择图片文件', false)
    return
  }
  if (file.size > 2 * 1024 * 1024) {
    showToast('图片大小不能超过 2MB', false)
    return
  }
  uploadingAvatar.value = true
  try {
    const res: any = await uploadAvatar(file)
    avatarUrl.value = res.avatar_url
    // 同步到 store
    if (userStore.userInfo) {
      userStore.userInfo.avatar_url = res.avatar_url
      localStorage.setItem('fake_bili_user', JSON.stringify(userStore.userInfo))
    }
    showToast('头像已更新')
  } catch {
    showToast('头像上传失败', false)
  } finally {
    uploadingAvatar.value = false
    // 清 input.value 以便下次选同一张图也能触发 change
    input.value = ''
  }
}

async function handleSave() {
  if (!form.value.username.trim()) {
    showToast('用户名不能为空', false)
    return
  }
  if (form.value.username.length > 20) {
    showToast('用户名最多 20 字符', false)
    return
  }
  if (form.value.signature.length > 320) {
    showToast('签名最多 320 字符', false)
    return
  }
  saving.value = true
  try {
    await changeUserInfo({
      username: form.value.username,
      signature: form.value.signature,
      gender: form.value.gender,
      birthday: form.value.birthday || '',
    })
    // 同步到 store + localStorage
    if (userStore.userInfo) {
      userStore.userInfo.username = form.value.username
      userStore.userInfo.signature = form.value.signature
      userStore.userInfo.gender = form.value.gender
      userStore.userInfo.birthday = form.value.birthday
      localStorage.setItem('fake_bili_user', JSON.stringify(userStore.userInfo))
    }
    showToast('保存成功')
  } catch {
    // 错误已由拦截器 toast
  } finally {
    saving.value = false
  }
}

onMounted(loadProfile)
</script>

<template>
  <div class="bg-white rounded-card shadow-card p-6">
    <h2 class="text-lg font-medium text-ink mb-6">个人资料</h2>

    <!-- 头像区 -->
    <div class="flex items-center gap-4 pb-6 mb-6 border-b border-surface-muted">
      <div class="relative">
        <img
          :src="avatarUrl || '/images/avatar-default.jpg'"
          alt="avatar"
          class="w-20 h-20 rounded-full object-cover border border-surface-muted"
          @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
        />
        <label
          class="absolute inset-0 flex items-center justify-center bg-black/40 text-white rounded-full opacity-0 hover:opacity-100 cursor-pointer transition"
          :class="{ '!opacity-100': uploadingAvatar }"
        >
          <Loader2 v-if="uploadingAvatar" :size="22" class="animate-spin" />
          <Camera v-else :size="22" />
          <input
            type="file"
            accept="image/jpeg,image/png,image/gif,image/webp"
            class="hidden"
            @change="handleAvatarChange"
          />
        </label>
      </div>
      <div>
        <div class="text-sm text-ink">头像</div>
        <div class="text-xs text-ink-muted mt-1">点击图片上传 · jpg/png/gif/webp · ≤2MB</div>
      </div>
    </div>

    <!-- 表单 -->
    <div class="space-y-5 max-w-xl">
      <div>
        <label class="block text-sm text-ink-secondary mb-1.5">用户名</label>
        <input
          v-model="form.username"
          type="text"
          maxlength="20"
          placeholder="1-20 字符"
          class="w-full h-10 px-3 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
        />
      </div>

      <div>
        <label class="block text-sm text-ink-secondary mb-1.5">签名</label>
        <textarea
          v-model="form.signature"
          maxlength="320"
          rows="3"
          placeholder="说点什么吧..."
          class="w-full px-3 py-2 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 resize-none"
        />
        <div class="text-right text-xs text-ink-muted mt-1">
          {{ form.signature.length }} / 320
        </div>
      </div>

      <div class="flex gap-6">
        <div class="flex-1">
          <label class="block text-sm text-ink-secondary mb-1.5">性别</label>
          <div class="flex gap-4 text-sm">
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input v-model="form.gender" type="radio" value="male" class="text-primary" />
              <span>男</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input v-model="form.gender" type="radio" value="female" class="text-primary" />
              <span>女</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input v-model="form.gender" type="radio" value="secret" class="text-primary" />
              <span>保密</span>
            </label>
          </div>
        </div>
        <div class="flex-1">
          <label class="block text-sm text-ink-secondary mb-1.5">生日</label>
          <input
            v-model="form.birthday"
            type="date"
            class="w-full h-10 px-3 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          />
        </div>
      </div>

      <div class="pt-2">
        <button
          :disabled="saving"
          class="h-10 px-6 bg-primary text-white rounded-lg text-sm font-medium hover:bg-primary-dark transition disabled:opacity-60"
          @click="handleSave"
        >
          {{ saving ? '保存中...' : '保存修改' }}
        </button>
      </div>
    </div>
  </div>
</template>
