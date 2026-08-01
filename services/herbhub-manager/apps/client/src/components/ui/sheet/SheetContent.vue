<script setup lang="ts">
import { DialogContent, DialogClose, type DialogContentProps } from 'reka-ui'
import { X } from '@lucide/vue'
import { cn } from '@/lib/utils'
import SheetPortal from './SheetPortal.vue'
import SheetOverlay from './SheetOverlay.vue'

const props = withDefaults(defineProps<DialogContentProps & { class?: string; side?: 'left' | 'right' }>(), {
  side: 'right',
})
</script>

<template>
  <SheetPortal>
    <SheetOverlay />
    <DialogContent
      :class="
        cn(
          'fixed z-50 bg-background p-4 shadow-lg transition ease-in-out data-[state=closed]:duration-200 data-[state=open]:duration-300 inset-y-0 h-full w-3/4 border-l sm:max-w-sm',
          side === 'left' ? 'left-0 border-r border-l-0' : 'right-0',
          props.class,
        )
      "
    >
      <slot />
      <DialogClose class="absolute right-4 top-4 rounded-sm opacity-70 hover:opacity-100 focus:outline-none">
        <X class="h-4 w-4" />
        <span class="sr-only">Close</span>
      </DialogClose>
    </DialogContent>
  </SheetPortal>
</template>
