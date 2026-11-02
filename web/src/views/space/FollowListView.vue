<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UserPlus, UserCheck, Users } from 'lucide-vue-next'
import { getFollowers, getFollowing } from '@/api/follow'
import { followUser, unfollowUser } from '@/api/interaction'
import { useUserStore } from '@/stores/user'
import type { FollowUserItem, PaginatedResp } from '@/types'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'

const props = defineProps<{
  mode: 'followers' | 'following'
}>()

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const userId = computed(() => (route.query.user_id as string) || userStore.userInfo?.id || '')
const list = ref<FollowUserItem[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const followLoadingId = ref<string | null>(null)
const pageSize = 20

const title = computed(() => (props.mode === 'followers' ? '我的粉丝' : '我的关注'))

async function load() {
  if (!userId.value) return
  loading.value = true
  try {
    const api = props.mode === 'followers' ? getFollowers : getFollowing
    const res: PaginatedResp<FollowUserItem> = await api(userId.value, page.value, pageSize)
    // following 列表中的用户都是已关注状态
    list.value = (res.list || []).map(item => ({
      ...item,
      is_followed: props.mode === 'following',
    }))
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

async function toggleFollow(item: FollowUserItem) {
  if (item.id === userStore.userInfo?.id) return // 不能关注自己
  followLoadingId.value = item.id
  try {
    if (item.is_followed) {
      await unfollowUser(item.id)
      item.is_followed = false
    } else {
      await followUser(item.id)
      item.is_followed = true
    }
  } finally {
    followLoadingId.value = null
  }
}

function goToUser(id: string) {
  router.push(`/user/${id}`)
}

onMounted(load)
watch(() => props.mode, load)
watch(() => page.value, load)
</script>

<template>
  <div class="bg-white rounded-card shadow-card p-6">
    <div class="flex items-center gap-2 mb-4">
      <Users :size="20" class="text-primary" />
      <h2 class="text-lg font-medium text-ink">{{ title }}</h2>
      <span class="text-sm text-ink-muted ml-2">共 {{ total }} 人</span>
    </div>

    <div v-if="loading" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
    <EmptyState v-else-if="!list.length" :text="mode === 'followers' ? '还没有粉丝' : '还没有关注任何人'" />

    <template v-else>
      <div class="divide-y divide-surface-muted">
        <div
          v-for="item in list"
          :key="item.id"
          class="flex items-center gap-3 py-3"
        >
          <img
            :src="item.avatar_url || '/images/avatar-default.jpg'"
            :alt="item.username"
            class="w-12 h-12 rounded-full object-cover cursor-pointer"
            @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
            @click="goToUser(item.id)"
          />
          <div class="flex-1 min-w-0 cursor-pointer" @click="goToUser(item.id)">
            <div class="text-sm font-medium text-ink truncate">{{ item.username }}</div>
            <div class="text-xs text-ink-muted truncate mt-0.5">
              {{ item.signature || '这个人很懒，什么都没写~' }}
            </div>
          </div>
          <button
            v-if="item.id !== userStore.userInfo?.id"
            class="flex items-center gap-1 px-3 h-8 rounded text-xs font-medium transition disabled:opacity-60"
            :class="item.is_followed
              ? 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
              : 'bg-primary text-white hover:bg-primary-dark'"
            :disabled="followLoadingId === item.id"
            @click="toggleFollow(item)"
          >
            <component :is="item.is_followed ? UserCheck : UserPlus" :size="14" />
            {{ item.is_followed ? '已关注' : '关注' }}
          </button>
        </div>
      </div>
      <Pagination :current="page" :total="total" :page-size="pageSize" @change="(p: number) => (page = p)" />
    </template>
  </div>
</template>
