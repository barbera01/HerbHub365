<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { ExternalLink } from '@lucide/vue'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import StatePanel from '@/components/StatePanel.vue'
import { useManagerStore } from '@/stores/manager'
import { apiFetchBlob } from '@/api/client'

const store = useManagerStore()
const objectUrls = new Set<string>()

onMounted(() => void store.loadVideos())

onUnmounted(() => {
  for (const u of objectUrls) URL.revokeObjectURL(u)
  objectUrls.clear()
})

function trackURL(blob: Blob): string {
  const u = URL.createObjectURL(blob)
  objectUrls.add(u)
  return u
}

async function playVideo(name: string) {
  const blob = await apiFetchBlob(`/api/videos/${encodeURIComponent(name)}`)
  const url = trackURL(blob)
  window.open(url, '_blank', 'noopener,noreferrer')
}

async function downloadVideo(name: string) {
  const blob = await apiFetchBlob(`/api/videos/${encodeURIComponent(name)}`)
  const url = trackURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
}
</script>

<template>
  <div class="space-y-4">
    <header class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold">Videos</h1>
        <p class="text-sm text-muted-foreground">Generated MP4 output library.</p>
      </div>
      <Button variant="outline" @click="store.loadVideos">Refresh</Button>
    </header>

    <StatePanel v-if="store.videos.length === 0" title="No videos" message="No generated videos found in output directory." kind="empty" />

    <div v-else class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      <Card v-for="video in store.videos" :key="video.name">
        <CardHeader>
          <CardTitle class="truncate">{{ video.name }}</CardTitle>
          <CardDescription>{{ video.size_mb }} · {{ video.modified }}</CardDescription>
        </CardHeader>
        <CardContent>
          <p class="text-sm text-muted-foreground">Protected media requires an authenticated fetch.</p>
        </CardContent>
        <CardFooter class="justify-end gap-2">
          <Button size="sm" variant="outline" @click="playVideo(video.name)">
            <ExternalLink class="h-4 w-4" /> Play
          </Button>
          <Button size="sm" variant="outline" @click="downloadVideo(video.name)">Download</Button>
        </CardFooter>
      </Card>
    </div>
  </div>
</template>
