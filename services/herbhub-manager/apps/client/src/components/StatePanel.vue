<script setup lang="ts">
defineProps<{ title: string; message: string; kind?: 'loading' | 'empty' | 'error' }>()
</script>

<template>
  <div
    class="flex min-h-24 flex-col items-center justify-center rounded-md border border-dashed p-6 text-center text-sm"
    :class="kind === 'error' ? 'border-red-300 text-red-700' : 'text-muted-foreground'"
  >
    <p class="font-semibold">{{ title }}</p>
    <p>{{ message }}</p>
  </div>
</template>
