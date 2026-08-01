<script setup lang="ts">
import { cn } from '@/lib/utils'
defineProps<{ modelValue?: string; class?: string }>()
defineEmits<{ 'update:modelValue': [string] }>()
</script>

<template>
  <select
    :value="modelValue"
    :class="cn('flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm', $props.class)"
    @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <slot />
  </select>
</template>
