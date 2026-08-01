<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import StatePanel from '@/components/StatePanel.vue'
import { useManagerStore } from '@/stores/manager'
import { timeAgo } from '@/lib/utils'

const store = useManagerStore()
const sortedJobs = computed(() => [...store.jobs].sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1)))
const error = computed(() => store.error)

let timer: number | undefined

onMounted(async () => {
  await store.loadJobsSafe()
  timer = window.setInterval(() => void store.loadJobs(), 3000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})

async function cancelQueue(id: string) {
  await store.cancelQueueItem(id)
  await store.loadJobs()
}
</script>

<template>
  <div class="space-y-4">
    <header class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold">Jobs</h1>
        <p class="text-sm text-muted-foreground">Queue + narrator pipeline progress and execution context.</p>
      </div>
      <Button variant="outline" @click="store.loadJobs">Refresh</Button>
    </header>

    <p v-if="error" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">{{ error }}</p>

    <StatePanel v-if="store.queueItems.length === 0 && sortedJobs.length === 0" title="No jobs" message="Queue posts from Posts view." kind="empty" />

    <div class="grid gap-3">
      <Card v-for="item in store.queueItems" :key="item.id">
        <CardHeader>
          <CardTitle>{{ item.title || item.slug }}</CardTitle>
          <CardDescription>{{ item.slug }} · queued {{ timeAgo(item.created_at) }}</CardDescription>
        </CardHeader>
        <CardContent>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex flex-wrap gap-2">
              <Badge variant="warning">Queue {{ item.phase }}</Badge>
              <Badge variant="outline">avatar: {{ item.avatar_id || 'default' }}</Badge>
              <Badge :variant="item.concat_enabled ? 'success' : 'secondary'">concat: {{ item.concat_enabled ? 'on' : 'off' }}</Badge>
              <Badge :variant="item.chroma_key_enabled ? 'success' : 'secondary'">chroma: {{ item.chroma_key_enabled ? 'on' : 'off' }}</Badge>
              <Badge v-if="item.concat_intro" variant="outline">intro: {{ item.concat_intro }}</Badge>
              <Badge v-if="item.concat_outro" variant="outline">outro: {{ item.concat_outro }}</Badge>
            </div>
            <Button v-if="item.phase === 'pending'" size="sm" variant="destructive" @click="cancelQueue(item.id)">Cancel</Button>
          </div>
        </CardContent>
      </Card>

      <Card v-for="job in sortedJobs" :key="job.id">
        <CardHeader>
          <CardTitle>{{ job.slug || job.id }}</CardTitle>
          <CardDescription>
            Updated {{ timeAgo(job.updated_at) }} · created {{ timeAgo(job.created_at) }}
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <Badge :variant="job.phase === 'completed' ? 'success' : job.phase === 'failed' ? 'danger' : 'secondary'">
              {{ job.phase }}
            </Badge>
            <span class="text-xs text-muted-foreground">{{ Math.round(job.progress * 100) }}%</span>
          </div>
          <div class="h-2 w-full rounded bg-muted">
            <div class="h-2 rounded bg-primary transition-all" :style="{ width: `${Math.round(job.progress * 100)}%` }" />
          </div>
          <div class="flex flex-wrap gap-2 text-xs">
            <Badge variant="outline">avatar: {{ job.avatar_id || 'default' }}</Badge>
            <Badge :variant="job.concat_enabled ? 'success' : 'secondary'">concat: {{ job.concat_enabled ? 'on' : 'off' }}</Badge>
            <Badge :variant="job.chroma_key_enabled ? 'success' : 'secondary'">chroma: {{ job.chroma_key_enabled ? 'on' : 'off' }}</Badge>
            <Badge v-if="job.concat_intro" variant="outline">intro: {{ job.concat_intro }}</Badge>
            <Badge v-if="job.concat_outro" variant="outline">outro: {{ job.concat_outro }}</Badge>
            <Badge v-if="job.chroma_key_bg" variant="outline">bg: {{ job.chroma_key_bg }}</Badge>
            <Badge v-if="job.video_file" variant="outline">file: {{ job.video_file }}</Badge>
          </div>
          <p v-if="job.error" class="text-sm text-red-700">{{ job.error }}</p>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
