<script setup lang="ts">
import { DialogOverlay, type DialogOverlayProps } from 'reka-ui'
import { cn } from '@/lib/utils'

defineProps<DialogOverlayProps & { class?: string }>()
</script>

<template>
  <DialogOverlay
    :class="cn('fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=closed]:animate-out', $props.class)"
  />
</template>
