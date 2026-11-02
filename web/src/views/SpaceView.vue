<script setup lang="ts">
import { UserCog, Users, UserPlus } from 'lucide-vue-next'

const navItems = [
  { to: '/space/profile', label: '个人资料', icon: UserCog },
  { to: '/space/followers', label: '我的粉丝', icon: Users },
  { to: '/space/following', label: '我的关注', icon: UserPlus },
]
</script>

<template>
  <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 py-4">
    <div class="flex gap-4">
      <!-- 左侧侧边栏 -->
      <aside class="w-48 shrink-0">
        <div class="bg-white rounded-card shadow-card p-2 sticky top-[72px]">
          <router-link
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="relative flex items-center gap-2 px-3 h-10 rounded text-sm transition-colors text-ink-secondary hover:bg-surface-subtle"
            active-class="!text-primary !bg-primary/5 font-medium"
          >
            <span
              v-if="$route.path === item.to"
              class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-5 bg-primary rounded-r"
            ></span>
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
          </router-link>
        </div>
      </aside>

      <!-- 右侧内容区 -->
      <main class="flex-1 min-w-0">
        <router-view />
      </main>
    </div>
  </div>
</template>
