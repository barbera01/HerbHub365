import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/session/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/signin', component: () => import('@/views/SignInView.vue'), meta: { public: true } },
    { path: '/', redirect: '/posts' },
    { path: '/posts', component: () => import('@/views/PostsView.vue') },
    { path: '/blog', component: () => import('@/views/BlogView.vue') },
    { path: '/jobs', component: () => import('@/views/JobsView.vue') },
    { path: '/messaging', component: () => import('@/views/MessagingView.vue') },
    { path: '/timelapse', component: () => import('@/views/TimelapseView.vue') },
    { path: '/videos', component: () => import('@/views/VideosView.vue') },
    { path: '/settings', component: () => import('@/views/SettingsView.vue') },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.ready) {
    await auth.initialise()
  }

  if (auth.authDisabled) return true

  if (to.meta.public) return true
  if (!auth.isAuthenticated) {
    return '/signin'
  }
  return true
})

export default router
