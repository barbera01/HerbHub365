<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ExternalLink, ListChecks, Play, Send } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import StatePanel from '@/components/StatePanel.vue'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { apiFetch, apiFetchBlob } from '@/api/client'
import { useManagerStore } from '@/stores/manager'

const store = useManagerStore()
const search = ref('')
const selectMode = ref(false)
const busy = ref(false)
const error = ref('')
const postsStatus = ref('')
const configStatus = ref('')
const resourcesStatus = ref('')

const pageSize = 36
const currentPage = ref(1)

const avatarId = ref('')
const textOverride = ref('')
const concatEnabled = ref(true)
const concatIntro = ref('')
const concatOutro = ref('')
const chromaKeyEnabled = ref(false)
const chromaKeyBg = ref('')

const generating = ref(false)
const generateOpen = ref(false)
const selectedSlug = ref('')

const objectUrls = new Set<string>()
const playUrls = new Map<string, string>()

const filteredPosts = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return store.posts
  return store.posts.filter((p) => p.title.toLowerCase().includes(q) || p.slug.toLowerCase().includes(q))
})

const totalResults = computed(() => filteredPosts.value.length)
const totalPages = computed(() => Math.max(1, Math.ceil(totalResults.value / pageSize)))
const pagedPosts = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredPosts.value.slice(start, start + pageSize)
})

watch(search, () => {
  currentPage.value = 1
})

watch(totalPages, (pages) => {
  if (currentPage.value > pages) currentPage.value = pages
})

function resetOptions() {
  const cfg = store.config
  avatarId.value = cfg?.default_avatar ?? 'eve'
  concatEnabled.value = cfg?.concat_enabled !== false
  chromaKeyEnabled.value = cfg?.chroma_key_enabled === true
  concatIntro.value = ''
  concatOutro.value = ''
  chromaKeyBg.value = ''
  textOverride.value = ''
}

function openGenerate(slug: string) {
  selectedSlug.value = slug
  generateOpen.value = true
  resetOptions()
}

function closeGenerate() {
  generateOpen.value = false
  selectedSlug.value = ''
  generating.value = false
}

async function submitGenerate() {
  if (!selectedSlug.value) return
  generating.value = true
  error.value = ''
  try {
    await apiFetch('/api/generate', {
      method: 'POST',
      body: {
        slug: selectedSlug.value,
        avatar_id: avatarId.value,
        text: textOverride.value || undefined,
        concat_enabled: concatEnabled.value,
        concat_intro: concatIntro.value || undefined,
        concat_outro: concatOutro.value || undefined,
        chroma_key_enabled: chromaKeyEnabled.value,
        chroma_key_bg: chromaKeyBg.value || undefined,
      },
    })
    closeGenerate()
    await store.loadJobs()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    generating.value = false
  }
}

async function queueSelected() {
  busy.value = true
  error.value = ''
  try {
    await store.queueSelection({
      textOverride: textOverride.value,
      avatarId: avatarId.value,
      concatEnabled: concatEnabled.value,
      concatIntro: concatIntro.value,
      concatOutro: concatOutro.value,
      chromaKeyEnabled: chromaKeyEnabled.value,
      chromaKeyBg: chromaKeyBg.value,
    })
    store.clearSelected()
    selectMode.value = false
    resetOptions()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function publish(slug: string) {
  busy.value = true
  error.value = ''
  try {
    await store.publish(slug)
    await store.loadPosts()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function createObjectURL(blob: Blob): string {
  const url = URL.createObjectURL(blob)
  objectUrls.add(url)
  return url
}

async function playVideo(filename: string) {
  if (!filename) return
  const previous = playUrls.get(filename)
  if (previous) {
    window.setTimeout(() => {
      URL.revokeObjectURL(previous)
      objectUrls.delete(previous)
    }, 60_000)
  }
  const blob = await apiFetchBlob(`/api/videos/${encodeURIComponent(filename)}`)
  const url = createObjectURL(blob)
  playUrls.set(filename, url)
  window.open(url, '_blank', 'noopener,noreferrer')
}

async function downloadVideo(filename: string) {
  if (!filename) return
  const blob = await apiFetchBlob(`/api/videos/${encodeURIComponent(filename)}`)
  const url = createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  window.setTimeout(() => {
    URL.revokeObjectURL(url)
    objectUrls.delete(url)
  }, 1_000)
}

async function loadSections() {
  postsStatus.value = ''
  configStatus.value = ''
  resourcesStatus.value = ''

  const [postsResult, configResult, resourcesResult] = await Promise.allSettled([
    store.loadPosts(),
    store.loadConfig(),
    store.loadResources(),
  ])

  if (postsResult.status === 'rejected') {
    postsStatus.value = (postsResult.reason as Error)?.message || 'Failed to load posts'
  }
  if (configResult.status === 'rejected') {
    configStatus.value = (configResult.reason as Error)?.message || 'Failed to load config'
  }
  if (resourcesResult.status === 'rejected') {
    resourcesStatus.value = (resourcesResult.reason as Error)?.message || 'Resource presets unavailable'
  }

  resetOptions()
}

onMounted(() => {
  void loadSections()
})

onUnmounted(() => {
  for (const url of objectUrls) URL.revokeObjectURL(url)
  objectUrls.clear()
})
</script>

<template>
  <div class="space-y-4">
    <header class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-lg font-semibold">Posts</h1>
        <p class="text-sm text-muted-foreground">Direct generate + bulk queue + publish workflow.</p>
        <p class="text-xs text-muted-foreground">
          Showing {{ pagedPosts.length }} of {{ totalResults }} posts (page {{ currentPage }} / {{ totalPages }})
        </p>
      </div>
      <div class="flex gap-2">
        <Input v-model="search" placeholder="Search by title or slug" class="w-64" />
        <Button variant="outline" @click="selectMode = !selectMode">
          <ListChecks class="h-4 w-4" />
          {{ selectMode ? 'Exit bulk mode' : 'Bulk queue' }}
        </Button>
      </div>
    </header>

    <StatePanel v-if="busy" title="Working" message="Applying operation..." kind="loading" />
    <p v-if="error" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ error }}</p>
    <p v-if="postsStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Posts: {{ postsStatus }}</p>
    <p v-if="configStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Config: {{ configStatus }}</p>
    <p v-if="resourcesStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Resources: {{ resourcesStatus }}</p>

    <Card v-if="selectMode">
      <CardHeader>
        <CardTitle>Bulk queue options</CardTitle>
        <CardDescription>Selected: {{ store.selected.size }}. Text override applies to all selected posts.</CardDescription>
      </CardHeader>
      <CardContent class="grid gap-3 md:grid-cols-2">
        <div class="space-y-1">
          <label class="text-xs font-semibold">Avatar</label>
          <Select v-model="avatarId">
            <option v-for="a in store.config?.avatars ?? ['eve']" :key="a" :value="a">{{ a }}</option>
          </Select>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <label class="col-span-1 flex items-center gap-2 text-sm"><input v-model="concatEnabled" type="checkbox" /> Concat</label>
          <label class="col-span-1 flex items-center gap-2 text-sm"><input v-model="chromaKeyEnabled" type="checkbox" /> Chroma key</label>
        </div>
        <div class="space-y-1">
          <label class="text-xs font-semibold">Intro</label>
          <Select v-model="concatIntro">
            <option value="">Server default</option>
            <option v-for="i in store.resources.intros" :key="i" :value="i">{{ i }}</option>
          </Select>
        </div>
        <div class="space-y-1">
          <label class="text-xs font-semibold">Outro</label>
          <Select v-model="concatOutro">
            <option value="">Server default</option>
            <option v-for="o in store.resources.outros" :key="o" :value="o">{{ o }}</option>
          </Select>
        </div>
        <div class="space-y-1">
          <label class="text-xs font-semibold">Background</label>
          <Select v-model="chromaKeyBg">
            <option value="">Server default</option>
            <option v-for="b in store.resources.backgrounds" :key="b" :value="b">{{ b }}</option>
          </Select>
        </div>
        <div class="space-y-1 md:col-span-2">
          <label class="text-xs font-semibold">Text override (optional)</label>
          <Textarea v-model="textOverride" :rows="4" placeholder="Leave blank to use post content." />
        </div>
      </CardContent>
      <CardFooter class="justify-end gap-2">
        <Button variant="ghost" @click="store.clearSelected">Clear selection</Button>
        <Button :disabled="store.selected.size === 0 || busy" @click="queueSelected"><Send class="h-4 w-4" /> Queue {{ store.selected.size }}</Button>
      </CardFooter>
    </Card>

    <div class="flex items-center justify-end text-sm text-muted-foreground">
      <div class="flex items-center gap-2" role="navigation" aria-label="Posts pagination">
        <Button variant="outline" size="sm" :disabled="currentPage <= 1" aria-label="Previous page" @click="currentPage -= 1">Previous</Button>
        <Button variant="outline" size="sm" :disabled="currentPage >= totalPages" aria-label="Next page" @click="currentPage += 1">Next</Button>
      </div>
    </div>

    <StatePanel v-if="filteredPosts.length === 0" title="No posts" message="No matching posts found in BLOG_POSTS_DIR." kind="empty" />

    <div v-else class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      <Card v-for="post in pagedPosts" :key="post.slug" class="border transition hover:border-primary" :class="store.selected.has(post.slug) ? 'ring-2 ring-primary' : ''">
        <CardHeader>
          <div class="flex items-start justify-between gap-2">
            <CardTitle>{{ post.title }}</CardTitle>
            <label v-if="selectMode" class="pt-0.5">
              <input type="checkbox" :checked="store.selected.has(post.slug)" @change="store.toggleSelected(post.slug)" :aria-label="`Select ${post.title}`" />
            </label>
          </div>
          <CardDescription>{{ post.date }} · {{ post.slug }}</CardDescription>
        </CardHeader>
        <CardContent>
          <p class="line-clamp-3 text-sm text-muted-foreground">{{ post.excerpt }}</p>
        </CardContent>
        <CardFooter class="justify-between gap-2">
          <Badge :variant="post.has_video ? 'success' : post.published ? 'secondary' : 'warning'">
            {{ post.has_video ? 'Video ready' : post.published ? 'Published' : 'No video' }}
          </Badge>
          <div class="flex flex-wrap gap-2">
            <a v-if="post.youtube_url" :href="post.youtube_url" target="_blank" rel="noreferrer">
              <Button size="sm" variant="ghost"><ExternalLink class="h-4 w-4" /> YouTube</Button>
            </a>
            <Button v-if="post.has_video" size="sm" variant="outline" @click="playVideo(post.video_file || '')"><Play class="h-4 w-4" /> Play</Button>
            <Button v-if="post.has_video" size="sm" variant="outline" @click="downloadVideo(post.video_file || '')">Download</Button>
            <Button v-if="post.has_video && !post.published && !selectMode" size="sm" variant="outline" @click="publish(post.slug)">Publish</Button>
            <Button v-if="!selectMode" size="sm" @click="openGenerate(post.slug)">Generate</Button>
          </div>
        </CardFooter>
      </Card>
    </div>

    <Sheet :open="generateOpen" @update:open="(v) => (generateOpen = v)">
      <SheetContent side="right" class="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Generate video</SheetTitle>
        </SheetHeader>
        <div class="space-y-3">
          <div class="space-y-1">
            <label class="text-xs font-semibold">Avatar</label>
            <Select v-model="avatarId">
              <option v-for="a in store.config?.avatars ?? ['eve']" :key="a" :value="a">{{ a }}</option>
            </Select>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <label class="col-span-1 flex items-center gap-2 text-sm"><input v-model="concatEnabled" type="checkbox" /> Concat</label>
            <label class="col-span-1 flex items-center gap-2 text-sm"><input v-model="chromaKeyEnabled" type="checkbox" /> Chroma key</label>
          </div>
          <div class="space-y-1">
            <label class="text-xs font-semibold">Intro</label>
            <Select v-model="concatIntro">
              <option value="">Server default</option>
              <option v-for="i in store.resources.intros" :key="i" :value="i">{{ i }}</option>
            </Select>
          </div>
          <div class="space-y-1">
            <label class="text-xs font-semibold">Outro</label>
            <Select v-model="concatOutro">
              <option value="">Server default</option>
              <option v-for="o in store.resources.outros" :key="o" :value="o">{{ o }}</option>
            </Select>
          </div>
          <div class="space-y-1">
            <label class="text-xs font-semibold">Background</label>
            <Select v-model="chromaKeyBg">
              <option value="">Server default</option>
              <option v-for="b in store.resources.backgrounds" :key="b" :value="b">{{ b }}</option>
            </Select>
          </div>
          <div class="space-y-1">
            <label class="text-xs font-semibold">Text override</label>
            <Textarea v-model="textOverride" :rows="5" />
          </div>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" @click="closeGenerate">Cancel</Button>
            <Button :disabled="generating" @click="submitGenerate">{{ generating ? 'Submitting...' : 'Generate' }}</Button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  </div>
</template>
