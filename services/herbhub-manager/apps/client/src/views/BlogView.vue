<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { apiFetch } from '@/api/client'

const prompt = ref('')
const categories = ref('Daily Update')
const systemPrompt = ref('')

const filename = ref('')
const title = ref('')
const content = ref('')
const saving = ref(false)
const error = ref('')
const generating = ref(false)
const loadingConfig = ref(true)
const configStatus = ref('')

async function generate() {
  error.value = ''
  generating.value = true
  try {
    const res = await apiFetch<{ filename: string; title: string; content: string }>('/api/blog/generate', {
      method: 'POST',
      body: {
        user_prompt: prompt.value || 'Write a daily herb garden update.',
        system_prompt: systemPrompt.value,
        categories: categories.value,
      },
    })
    filename.value = res.filename
    title.value = res.title
    content.value = res.content
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    generating.value = false
  }
}

async function loadBlogConfig() {
  loadingConfig.value = true
  configStatus.value = ''
  try {
    const cfg = await apiFetch<{
      system_prompt?: string
      site_name?: string
      site_url?: string
      plant_name?: string
    }>('/api/blog/config')
    if (!prompt.value) {
      prompt.value = `Write a daily ${cfg.plant_name || 'herb'} garden update blog post for ${cfg.site_name || 'HerbHub365'} (${cfg.site_url || 'https://herbhub365.com'}).`
    }
    if (!systemPrompt.value && cfg.system_prompt) {
      systemPrompt.value = cfg.system_prompt
    }
  } catch (e) {
    configStatus.value = (e as Error).message
  } finally {
    loadingConfig.value = false
  }
}

function discard() {
  filename.value = ''
  title.value = ''
  content.value = ''
}

void loadBlogConfig()

async function save() {
  error.value = ''
  saving.value = true
  try {
    await apiFetch('/api/blog/save', {
      method: 'POST',
      body: {
        filename: filename.value,
        content: content.value,
      },
    })
    filename.value = ''
    title.value = ''
    content.value = ''
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="grid gap-4 xl:grid-cols-[420px_1fr]">
    <Card>
      <CardHeader>
        <CardTitle>Blog Poster</CardTitle>
        <CardDescription>Generate and save Jekyll posts.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-3">
        <p v-if="loadingConfig" class="text-sm text-muted-foreground">Loading server blog defaults…</p>
        <p v-if="configStatus" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">Config: {{ configStatus }}</p>
        <p v-if="error" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ error }}</p>
        <div>
          <label class="mb-1 block text-xs font-semibold">Prompt</label>
          <Textarea v-model="prompt" :rows="4" placeholder="Topic or writing direction" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-semibold">Categories</label>
          <Input v-model="categories" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-semibold">System prompt override</label>
          <Textarea v-model="systemPrompt" :rows="6" />
        </div>
      </CardContent>
      <CardFooter>
        <Button :disabled="generating" @click="generate">Generate</Button>
      </CardFooter>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>{{ title || 'Preview' }}</CardTitle>
        <CardDescription>{{ filename || 'Generated markdown appears here.' }}</CardDescription>
      </CardHeader>
      <CardContent>
        <Textarea v-model="content" :rows="28" class="font-mono" />
      </CardContent>
      <CardFooter class="justify-end gap-2">
        <Button variant="ghost" :disabled="!filename && !content" @click="discard">Discard</Button>
        <Button :disabled="!filename || !content || saving" @click="save">Save to Posts</Button>
      </CardFooter>
    </Card>
  </div>
</template>
