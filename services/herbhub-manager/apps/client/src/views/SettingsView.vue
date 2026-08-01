<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import StatePanel from '@/components/StatePanel.vue'
import { useManagerStore } from '@/stores/manager'

const store = useManagerStore()
const loading = ref(true)
const error = ref('')

onMounted(async () => {
  loading.value = true
  error.value = ''
  try {
    await store.loadConfig()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="space-y-4">
    <header>
      <h1 class="text-lg font-semibold">Settings</h1>
      <p class="text-sm text-muted-foreground">Current API runtime status and config.</p>
    </header>

    <StatePanel v-if="loading" title="Loading" message="Loading manager configuration..." kind="loading" />
    <StatePanel v-else-if="error" title="Error" :message="error" kind="error" />
    <StatePanel v-else-if="!store.config" title="No config" message="No configuration returned by /api/config." kind="empty" />

    <div v-else class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      <Card>
        <CardHeader><CardTitle>Narrator URL</CardTitle></CardHeader>
        <CardContent class="break-all text-sm">{{ store.config?.narrator_url || 'N/A' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Narrator status</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.narrator_online ? 'Online' : 'Offline' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Default avatar</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.default_avatar || 'eve' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>MuseTalk URL</CardTitle></CardHeader>
        <CardContent class="break-all text-sm">{{ store.config?.musetalk_url || 'N/A' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Avatars</CardTitle><CardDescription>Configured options</CardDescription></CardHeader>
        <CardContent class="text-sm">{{ (store.config?.avatars || []).join(', ') || 'none' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Concat enabled</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.concat_enabled ? 'Yes' : 'No' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Chroma key enabled</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.chroma_key_enabled ? 'Yes' : 'No' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Posts directory</CardTitle></CardHeader>
        <CardContent class="break-all text-sm">{{ store.config?.posts_dir || 'N/A' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Output directory</CardTitle></CardHeader>
        <CardContent class="break-all text-sm">{{ store.config?.output_dir || 'N/A' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Poll interval</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.poll_interval || 'N/A' }}</CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Max wait</CardTitle></CardHeader>
        <CardContent class="text-sm">{{ store.config?.max_wait || 'N/A' }}</CardContent>
      </Card>
    </div>
  </div>
</template>
