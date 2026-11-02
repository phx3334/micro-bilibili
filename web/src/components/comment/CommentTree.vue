<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import type { CommentItem as CommentItemType } from '@/types'
import {
  getCommentList,
  getCommentReplies,
  createComment,
} from '@/api/comment'
import { useUserStore } from '@/stores/user'
import CommentItem from './CommentItem.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const props = defineProps<{
  videoId: number
  commentType: 'video'
}>()

const userStore = useUserStore()
const myAvatar = computed(() => userStore.userInfo?.avatar_url || '/images/avatar-default.jpg')

const comments = ref<CommentItemType[]>([])
const page = ref(1)
const total = ref(0)
const pageSize = 20
const loading = ref(false)
const submitting = ref(false)

// 顶级输入框
const inputContent = ref('')
// 当前回复目标：{ parentId: 被回复的评论 id, topId: 所属顶级评论 id, username: 被回复者名 }
const replyingTo = ref<{ parentId: number; topId: number; username: string } | null>(null)
const replyContent = ref('')
// 回复展开
const expandedReplies = reactive<Record<number, CommentItemType[]>>({})
const expandedSet = reactive<Set<number>>(new Set())
const replyLoading = ref<number | null>(null)

async function loadComments(p = 1) {
  loading.value = true
  try {
    const res = await getCommentList(props.commentType, props.videoId, p, pageSize)
    comments.value = res.list || []
    total.value = res.total || 0
    page.value = p
  } catch {
    comments.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function toggleReplies(parentId: number) {
  if (expandedSet.has(parentId)) {
    expandedSet.delete(parentId)
    return
  }
  replyLoading.value = parentId
  try {
    const res = await getCommentReplies(props.commentType, parentId, 1, 20)
    expandedReplies[parentId] = res.list || []
    expandedSet.add(parentId)
  } catch {
    // 加载回复失败，不展开
  } finally {
    replyLoading.value = null
  }
}

async function submitComment() {
  const content = inputContent.value.trim()
  if (!content || submitting.value) return
  submitting.value = true
  try {
    await createComment(props.commentType, props.videoId, content, 0)
    inputContent.value = ''
    await loadComments(1)
  } finally {
    submitting.value = false
  }
}

async function submitReply() {
  if (!replyingTo.value) return
  const content = replyContent.value.trim()
  if (!content || submitting.value) return
  submitting.value = true
  const target = replyingTo.value
  try {
    // parent_id 传被回复评论的 id，后端会自动计算 level
    await createComment(props.commentType, props.videoId, content, target.parentId)
    replyContent.value = ''
    replyingTo.value = null
    // 刷新该顶级评论下的回复列表
    const res = await getCommentReplies(props.commentType, target.topId, 1, 20)
    expandedReplies[target.topId] = res.list || []
    expandedSet.add(target.topId)
    // 本地增量更新顶级评论的 reply_count，避免全量重载
    const top = comments.value.find((c) => c.id === target.topId)
    if (top) {
      top.reply_count = (top.reply_count || 0) + 1
    }
  } catch {
    // 错误已由拦截器提示
  } finally {
    submitting.value = false
  }
}

// 顶级评论的回复入口
function startReply(parentId: number, username: string) {
  replyingTo.value = { parentId, topId: parentId, username }
  replyContent.value = ''
}

// 回复列表中某条回复的回复入口（楼中楼）
function startReplyToReply(replyId: number, topId: number, username: string) {
  replyingTo.value = { parentId: replyId, topId, username }
  replyContent.value = ''
}

function cancelReply() {
  replyingTo.value = null
  replyContent.value = ''
}

onMounted(() => {
  loadComments(1)
})
</script>

<template>
  <div>
    <!-- 评论输入框 -->
    <div class="flex gap-3 mb-4">
      <div class="relative shrink-0">
        <img
          :src="myAvatar"
          alt="me"
          class="w-9 h-9 rounded-full object-cover bg-surface-muted"
          @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
        />
      </div>
      <div class="flex-1 flex gap-2">
        <textarea
          v-model="inputContent"
          placeholder="发一条友善的评论吧"
          rows="2"
          class="flex-1 px-3 py-2 bg-surface-subtle rounded-card text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
          @keydown.ctrl.enter="submitComment"
        />
        <button
          class="px-4 self-end h-9 rounded-card bg-primary text-white text-sm hover:bg-primary-dark transition disabled:opacity-50"
          :disabled="!inputContent.trim() || submitting"
          @click="submitComment"
        >
          发送
        </button>
      </div>
    </div>

    <!-- 评论列表 -->
    <div v-if="loading && !comments.length" class="py-8 text-center text-sm text-ink-muted">
      加载中...
    </div>
    <EmptyState v-else-if="!comments.length" text="暂无评论，快来抢沙发" />
    <div v-else>
      <div v-for="comment in comments" :key="comment.id" :data-comment-id="comment.id">
        <CommentItem :comment="comment" :video-id="videoId" :target-type="commentType" @reply="(id) => startReply(id, comment.user?.username || '')" />

        <!-- 顶级评论下的回复输入框 -->
        <div v-if="replyingTo && replyingTo.topId === comment.id && replyingTo.parentId === comment.id" class="ml-12 mb-2 flex gap-2">
          <div class="relative shrink-0">
            <img
              :src="myAvatar"
              alt="me"
              class="w-7 h-7 rounded-full object-cover bg-surface-muted"
              @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
            />
          </div>
          <textarea
            v-model="replyContent"
            :placeholder="`回复 @${replyingTo.username}`"
            rows="2"
            class="flex-1 px-3 py-2 bg-surface-subtle rounded-card text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30"
          />
          <div class="flex flex-col gap-1 self-end">
            <button
              class="px-3 h-8 rounded bg-primary text-white text-xs hover:bg-primary-dark transition disabled:opacity-50"
              :disabled="!replyContent.trim() || submitting"
              @click="submitReply"
            >
              回复
            </button>
            <button
              class="px-3 h-8 rounded bg-surface-muted text-ink-secondary text-xs hover:bg-surface-muted/70 transition"
              @click="cancelReply"
            >
              取消
            </button>
          </div>
        </div>

        <!-- 展开回复 -->
        <div v-if="comment.reply_count > 0" class="ml-12">
          <button
            class="text-xs text-secondary hover:underline py-1"
            @click="toggleReplies(comment.id)"
          >
            {{ expandedSet.has(comment.id) ? '收起回复' : `查看 ${comment.reply_count} 条回复` }}
          </button>
          <div v-if="replyLoading === comment.id" class="text-xs text-ink-muted py-1">
            加载中...
          </div>
          <template v-if="expandedSet.has(comment.id)">
            <div
              v-for="reply in expandedReplies[comment.id]"
              :key="reply.id"
              class="border-l-2 border-surface-muted pl-3"
              :data-comment-id="reply.id"
            >
              <CommentItem :comment="reply" :video-id="videoId" :target-type="commentType" @reply="(id) => startReplyToReply(id, comment.id, reply.user?.username || '')" />
              <!-- 楼中楼回复输入框 -->
              <div v-if="replyingTo && replyingTo.parentId === reply.id && replyingTo.topId === comment.id" class="ml-9 mb-2 flex gap-2">
                <div class="relative shrink-0">
                  <img
                    :src="myAvatar"
                    alt="me"
                    class="w-6 h-6 rounded-full object-cover bg-surface-muted"
                    @error="($event.target as HTMLImageElement).src = '/images/avatar-default.jpg'"
                  />
                </div>
                <textarea
                  v-model="replyContent"
                  :placeholder="`回复 @${replyingTo.username}`"
                  rows="2"
                  class="flex-1 px-3 py-2 bg-surface-subtle rounded-card text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30"
                />
                <div class="flex flex-col gap-1 self-end">
                  <button
                    class="px-3 h-8 rounded bg-primary text-white text-xs hover:bg-primary-dark transition disabled:opacity-50"
                    :disabled="!replyContent.trim() || submitting"
                    @click="submitReply"
                  >
                    回复
                  </button>
                  <button
                    class="px-3 h-8 rounded bg-surface-muted text-ink-secondary text-xs hover:bg-surface-muted/70 transition"
                    @click="cancelReply"
                  >
                    取消
                  </button>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>

      <Pagination
        :current="page"
        :total="total"
        :page-size="pageSize"
        @change="loadComments"
      />
    </div>
  </div>
</template>
