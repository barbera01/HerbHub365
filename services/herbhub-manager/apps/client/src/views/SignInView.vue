<script setup lang="ts">
import { Leaf } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/session/auth'

const auth = useAuthStore()
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-[#f2f6ea] p-4">
    <div class="w-full max-w-md rounded-lg border bg-card p-8 shadow">
      <div class="mb-4 flex items-center gap-2">
        <Leaf class="h-5 w-5 text-primary" />
        <h1 class="text-lg font-semibold">Herb Hub Manager</h1>
      </div>
      <p class="mb-6 text-sm text-muted-foreground">Sign in with your workforce account to continue.</p>
      <p v-if="auth.authDisabled" class="mb-4 rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-800">
        SPA auth is disabled via runtime config for local development.
      </p>
      <p v-if="auth.error" class="mb-4 rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ auth.error }}</p>
      <Button class="w-full" :disabled="auth.authDisabled" @click="auth.login">Sign in with Microsoft Entra ID</Button>
    </div>
  </div>
</template>
