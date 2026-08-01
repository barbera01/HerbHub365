import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { apiFetch } from '@/api/client'
import type { JobStatus, NarratorConfig, PostListItem, QueueItem, Resources, VideoFile } from '@/types/api'

export const useManagerStore = defineStore('manager', () => {
  const posts = ref<PostListItem[]>([])
  const jobs = ref<JobStatus[]>([])
  const queueItems = ref<QueueItem[]>([])
  const videos = ref<VideoFile[]>([])
  const resources = ref<Resources>({ intros: [], outros: [], backgrounds: [] })
  const config = ref<NarratorConfig | null>(null)
  const selected = ref<Set<string>>(new Set())
  const loading = ref(false)
  const error = ref('')

  const activeJobs = computed(() => jobs.value.filter((j) => !['completed', 'failed'].includes(j.phase)).length)
  const pendingQueue = computed(() => queueItems.value.filter((q) => q.phase === 'pending').length)

  async function loadPosts() {
    const data = await apiFetch<{ posts: PostListItem[] }>('/api/posts')
    posts.value = data.posts
  }

  async function loadPostsSafe() {
    try {
      await loadPosts()
      error.value = ''
    } catch (e) {
      error.value = (e as Error).message
    }
  }

  async function loadJobs() {
    const [jobsData, queueData] = await Promise.all([
      apiFetch<{ jobs: JobStatus[] }>('/api/jobs'),
      apiFetch<{ items: QueueItem[] }>('/api/queue'),
    ])
    jobs.value = jobsData.jobs
    queueItems.value = queueData.items
  }

  async function loadJobsSafe() {
    try {
      await loadJobs()
      error.value = ''
    } catch (e) {
      error.value = (e as Error).message
    }
  }

  async function loadVideos() {
    const data = await apiFetch<{ videos: VideoFile[] }>('/api/videos')
    videos.value = data.videos
  }

  async function loadConfig() {
    config.value = await apiFetch<NarratorConfig>('/api/config')
  }

  async function loadResources() {
    resources.value = await apiFetch<Resources>('/api/resources')
  }

  async function publish(slug: string) {
    await apiFetch('/api/publish', { method: 'POST', body: { slug } })
  }

  async function queueSelection(opts: {
    textOverride?: string
    avatarId?: string
    concatEnabled: boolean
    concatIntro?: string
    concatOutro?: string
    chromaKeyEnabled: boolean
    chromaKeyBg?: string
  }) {
    const slugs = Array.from(selected.value)
    await apiFetch('/api/queue', {
      method: 'POST',
      body: {
        slugs,
        text_override: opts.textOverride ?? '',
        avatar_id: opts.avatarId ?? '',
        concat_enabled: opts.concatEnabled,
        concat_intro: opts.concatIntro ?? '',
        concat_outro: opts.concatOutro ?? '',
        chroma_key_enabled: opts.chromaKeyEnabled,
        chroma_key_bg: opts.chromaKeyBg ?? '',
      },
    })
  }

  async function cancelQueueItem(id: string) {
    await apiFetch(`/api/queue/${id}`, { method: 'DELETE' })
  }

  function toggleSelected(slug: string) {
    const next = new Set(selected.value)
    if (next.has(slug)) next.delete(slug)
    else next.add(slug)
    selected.value = next
  }

  function clearSelected() {
    selected.value = new Set()
  }

  return {
    posts,
    jobs,
    queueItems,
    videos,
    resources,
    config,
    selected,
    loading,
    error,
    activeJobs,
    pendingQueue,
    loadPosts,
    loadPostsSafe,
    loadJobs,
    loadJobsSafe,
    loadVideos,
    loadConfig,
    loadResources,
    publish,
    queueSelection,
    cancelQueueItem,
    toggleSelected,
    clearSelected,
  }
})
