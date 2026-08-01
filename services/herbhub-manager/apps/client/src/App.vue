<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ManagerLayout from '@/components/ManagerLayout.vue'

const route = useRoute()
const isSignIn = computed(() => route.path === '/signin')
</script>

<template>
  <RouterView v-if="isSignIn" />
  <ManagerLayout v-else>
    <RouterView />
  </ManagerLayout>
</template>
