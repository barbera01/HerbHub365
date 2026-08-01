<script setup lang="ts">
import { cn } from '@/lib/utils'
import { badgeVariants } from './badgeVariants'

withDefaults(defineProps<{ variant?: 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'danger' }>(), {
  variant: 'default',
})
</script>

<template>
  <span :class="cn(badgeVariants({ variant }))">
    <slot />
  </span>
</template>
