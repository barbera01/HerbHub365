<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select } from '@/components/ui/select'
import { apiFetch, apiFetchBlob } from '@/api/client'
import type { TimelapseBuildJob, TimelapseNarrateJob } from '@/types/api'
import { timeAgo } from '@/lib/utils'

const status = ref<'loading' | 'online' | 'offline'>('loading')
const videos = ref<Array<{ name: string; size_mb: string; modified: string }>>([])
const jobs = ref<TimelapseBuildJob[]>([])
const publishJobs = ref<TimelapseNarrateJob[]>([])
const config = ref<Record<string, unknown>>({})

const from = ref('')
const to = ref('')
const outputName = ref('')
const inputFPS = ref('')
const outputFPS = ref('')
const crf = ref('')
const brightness = ref('')

const publishVideo = ref('')
const publishTitle = ref('')
const publishFromDate = ref('')
const publishToDate = ref('')
const publishScript = ref('')
const intro = ref('')
const outro = ref('')
const intros = ref<string[]>([])
const outros = ref<string[]>([])
const error = ref('')
const busy = ref(false)
const healthStatus = ref('')
const videosStatus = ref('')
const jobsStatus = ref('')
const resourcesStatus = ref('')
const configStatus = ref('')

let timer: number | undefined
const objectUrls = new Set<string>()

const sortedJobs = computed(() => [...jobs.value].sort((a, b) => (a.updated_at || '') < (b.updated_at || '') ? 1 : -1))

function formatDT(v: string): string {
  if (!v) return ''
  return v.replace('T', ' ') + ':00'
}

function trackObjectURL(blob: Blob): string {
  const url = URL.createObjectURL(blob)
  objectUrls.add(url)
  return url
}

async function downloadTimelapse(fileName: string) {
  if (!fileName) return
  const blob = await apiFetchBlob(`/api/timelapse/videos/${encodeURIComponent(fileName)}`)
  const url = trackObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  a.click()
}

async function load() {
  healthStatus.value = ''
  videosStatus.value = ''
  jobsStatus.value = ''
  resourcesStatus.value = ''
  configStatus.value = ''

  const [health, videosRes, jobsRes, resources, configRes] = await Promise.allSettled([
    apiFetch<{ online: boolean }>('/api/timelapse/health'),
    apiFetch<{ videos: Array<{ name: string; size_mb: string; modified: string }> }>('/api/timelapse/videos'),
    apiFetch<{ jobs: TimelapseBuildJob[] }>('/api/timelapse/jobs'),
    apiFetch<{ intros: string[]; outros: string[] }>('/api/resources'),
    apiFetch<Record<string, unknown>>('/api/timelapse/config'),
  ])

  if (health.status === 'fulfilled') {
    status.value = health.value.online ? 'online' : 'offline'
  } else {
    status.value = 'offline'
    healthStatus.value = (health.reason as Error)?.message || 'health unavailable'
  }

  if (videosRes.status === 'fulfilled') {
    videos.value = videosRes.value.videos ?? []
  } else {
    videosStatus.value = (videosRes.reason as Error)?.message || 'videos unavailable'
  }

  if (jobsRes.status === 'fulfilled') {
    jobs.value = jobsRes.value.jobs ?? []
  } else {
    jobsStatus.value = (jobsRes.reason as Error)?.message || 'jobs unavailable'
  }

  if (resources.status === 'fulfilled') {
    intros.value = resources.value.intros ?? []
    outros.value = resources.value.outros ?? []
  } else {
    resourcesStatus.value = (resources.reason as Error)?.message || 'resources unavailable'
  }

  if (configRes.status === 'fulfilled') {
    config.value = configRes.value
  } else {
    configStatus.value = (configRes.reason as Error)?.message || 'config unavailable'
  }
}

async function pollNarrationJobs() {
  const active = publishJobs.value.filter((j) => !['completed', 'failed'].includes(j.phase))
  await Promise.all(
    active.map(async (job) => {
      try {
        const updated = await apiFetch<TimelapseNarrateJob>(`/api/timelapse/narrate/${encodeURIComponent(job.id)}`)
        const idx = publishJobs.value.findIndex((x) => x.id === job.id)
        if (idx >= 0) publishJobs.value[idx] = updated
      } catch {
        // tolerate transient poll failures
      }
    }),
  )
}

async function build() {
  error.value = ''
  busy.value = true
  try {
    await apiFetch('/api/timelapse/build', {
      method: 'POST',
      body: {
        from: formatDT(from.value),
        to: formatDT(to.value),
        output_name: outputName.value,
        input_fps: inputFPS.value ? Number(inputFPS.value) : undefined,
        output_fps: outputFPS.value ? Number(outputFPS.value) : undefined,
        crf: crf.value ? Number(crf.value) : undefined,
        min_brightness: brightness.value ? Number(brightness.value) : undefined,
      },
    })
    await load()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function publish() {
  error.value = ''
  busy.value = true
  try {
    const queued = await apiFetch<{ job_id: string; slug: string; phase: string }>('/api/timelapse/publish', {
      method: 'POST',
      body: {
        timelapse_file: publishVideo.value,
        title: publishTitle.value,
        from_date: publishFromDate.value,
        to_date: publishToDate.value,
        tts_text: publishScript.value,
        intro: intro.value,
        outro: outro.value,
      },
    })
    publishJobs.value.unshift({
      id: queued.job_id,
      slug: queued.slug,
      phase: queued.phase,
      progress: 0,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    })
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  await load()
  timer = window.setInterval(async () => {
    await load()
    await pollNarrationJobs()
  }, 5000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  for (const url of objectUrls) URL.revokeObjectURL(url)
  objectUrls.clear()
})
</script>

<template>
  <div class="space-y-4">
    <header class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold">Timelapse</h1>
        <p class="text-sm text-muted-foreground">Build, narrate, and publish timelapse videos.</p>
      </div>
      <div class="text-xs" :class="status === 'online' ? 'text-emerald-700' : status === 'offline' ? 'text-red-700' : 'text-muted-foreground'">
        {{ status === 'loading' ? 'Checking...' : status === 'online' ? 'Online' : 'Offline' }}
      </div>
    </header>

    <p v-if="error" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ error }}</p>
    <p v-if="healthStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Health: {{ healthStatus }}</p>
    <p v-if="videosStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Videos: {{ videosStatus }}</p>
    <p v-if="jobsStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Jobs: {{ jobsStatus }}</p>
    <p v-if="resourcesStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Resources: {{ resourcesStatus }}</p>
    <p v-if="configStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Config: {{ configStatus }}</p>

    <div class="grid gap-4 xl:grid-cols-[420px_1fr]">
      <div class="space-y-4">
        <Card>
          <CardHeader><CardTitle>Build</CardTitle><CardDescription>Queue a new timelapse build.</CardDescription></CardHeader>
          <CardContent class="space-y-2">
            <Input v-model="from" type="datetime-local" />
            <Input v-model="to" type="datetime-local" />
            <Input v-model="outputName" placeholder="timelapse-today.mp4" />
            <div class="grid grid-cols-2 gap-2">
              <Input v-model="inputFPS" type="number" placeholder="Input FPS" />
              <Input v-model="outputFPS" type="number" placeholder="Output FPS" />
              <Input v-model="crf" type="number" placeholder="CRF" />
              <Input v-model="brightness" type="number" placeholder="Min brightness" />
            </div>
            <Button :disabled="busy" class="w-full" @click="build">Build timelapse</Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Publish</CardTitle><CardDescription>Narrate + publish timelapse video.</CardDescription></CardHeader>
          <CardContent class="space-y-2">
            <Select v-model="publishVideo">
              <option value="">Select timelapse file</option>
              <option v-for="v in videos" :key="v.name" :value="v.name">{{ v.name }}</option>
            </Select>
            <Input v-model="publishTitle" placeholder="Title" />
            <div class="grid grid-cols-2 gap-2">
              <Input v-model="publishFromDate" type="date" />
              <Input v-model="publishToDate" type="date" />
            </div>
            <Textarea v-model="publishScript" :rows="4" placeholder="Narration text" />
            <div class="grid grid-cols-2 gap-2">
              <Select v-model="intro">
                <option value="">Intro default</option>
                <option v-for="i in intros" :key="i" :value="i">{{ i }}</option>
              </Select>
              <Select v-model="outro">
                <option value="">Outro default</option>
                <option v-for="o in outros" :key="o" :value="o">{{ o }}</option>
              </Select>
            </div>
            <Button :disabled="busy" class="w-full" @click="publish">Publish to YouTube</Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Service config</CardTitle><CardDescription>/api/timelapse/config</CardDescription></CardHeader>
          <CardContent class="space-y-1 text-sm">
            <div v-for="(value, key) in config" :key="String(key)" class="flex justify-between gap-2 border-b py-1">
              <span class="font-medium">{{ key }}</span>
              <span class="text-muted-foreground">{{ String(value) }}</span>
            </div>
          </CardContent>
        </Card>
      </div>

      <div class="space-y-4">
        <Card>
          <CardHeader><CardTitle>Build jobs</CardTitle></CardHeader>
          <CardContent class="space-y-2">
            <div v-if="sortedJobs.length === 0" class="text-sm text-muted-foreground">No build jobs yet.</div>
            <div v-for="job in sortedJobs" :key="job.id" class="space-y-2 rounded border p-2 text-sm">
              <div class="flex items-center justify-between">
                <span class="font-medium">{{ job.output_file || job.id }}</span>
                <span class="text-muted-foreground">{{ job.status }}</span>
              </div>
              <div v-if="job.error" class="text-red-700">{{ job.error }}</div>
              <div class="text-xs text-muted-foreground">{{ timeAgo(job.updated_at || job.created_at) }}</div>
              <Button v-if="job.status === 'completed' && job.output_file" size="sm" variant="outline" @click="downloadTimelapse(job.output_file)">
                Download
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Narration/publish jobs</CardTitle></CardHeader>
          <CardContent class="space-y-2">
            <div v-if="publishJobs.length === 0" class="text-sm text-muted-foreground">No publish jobs yet.</div>
            <div v-for="job in publishJobs" :key="job.id" class="space-y-1 rounded border p-2 text-sm">
              <div class="flex items-center justify-between">
                <span class="font-medium">{{ job.slug || job.id }}</span>
                <span class="text-muted-foreground">{{ job.phase }}</span>
              </div>
              <div class="h-2 w-full rounded bg-muted">
                <div class="h-2 rounded bg-primary transition-all" :style="{ width: `${Math.round((job.progress || 0) * 100)}%` }" />
              </div>
              <div v-if="job.error" class="text-red-700">{{ job.error }}</div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Output videos</CardTitle></CardHeader>
          <CardContent class="space-y-2">
            <div v-if="videos.length === 0" class="text-sm text-muted-foreground">No timelapse videos yet.</div>
            <div v-for="video in videos" :key="video.name" class="flex items-center justify-between rounded border p-2 text-sm">
              <div>
                <div class="font-medium">{{ video.name }}</div>
                <div class="text-xs text-muted-foreground">{{ video.size_mb }} · {{ video.modified }}</div>
              </div>
              <Button size="sm" variant="outline" @click="downloadTimelapse(video.name)">Download</Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  </div>
</template>
