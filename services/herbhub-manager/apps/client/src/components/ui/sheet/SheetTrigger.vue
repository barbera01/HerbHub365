<script setup lang="ts">
import { DialogTrigger, type DialogTriggerProps } from 'reka-ui'

defineProps<DialogTriggerProps>()
</script>

<template>
  <DialogTrigger>
    <slot />
  </DialogTrigger>
</template>
