<script setup lang="ts">
import { cn } from '@/lib/utils'
import { buttonVariants } from './buttonVariants'

withDefaults(
  defineProps<{
    variant?: 'default' | 'secondary' | 'outline' | 'ghost' | 'destructive'
    size?: 'default' | 'sm' | 'lg'
    type?: 'button' | 'submit' | 'reset'
    disabled?: boolean
  }>(),
  {
    variant: 'default',
    size: 'default',
    type: 'button',
    disabled: false,
  },
)
</script>

<template>
  <button :type="type" :disabled="disabled" :class="cn(buttonVariants({ variant, size }))">
    <slot />
  </button>
</template>
