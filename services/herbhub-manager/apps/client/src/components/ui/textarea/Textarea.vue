<script setup lang="ts">
import { cn } from '@/lib/utils'
defineProps<{ modelValue?: string; rows?: number; placeholder?: string; class?: string }>()
defineEmits<{ 'update:modelValue': [string] }>()
</script>

<template>
  <textarea
    :rows="rows ?? 4"
    :value="modelValue"
    :placeholder="placeholder"
    :class="
      cn(
        'flex min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        $props.class,
      )
    "
    @input="$emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
  />
</template>
