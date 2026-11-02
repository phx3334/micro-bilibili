import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import router from './router'
import { useUserStore } from './stores/user'

const app = createApp(App)

app.use(createPinia())
app.use(router)

// 启动后若已登录，拉取最新用户信息，避免 localStorage 中的旧数据
const userStore = useUserStore()
if (userStore.isLogin) {
  userStore.fetchUserInfo().catch(() => {})
}

app.mount('#app')
