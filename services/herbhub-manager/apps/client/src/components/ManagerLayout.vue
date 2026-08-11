<script setup lang="ts">
import {
  CircleGauge,
  Clock3,
  FileText,
  MessagesSquare,
  Menu,
  MonitorCog,
  Settings,
  SquarePlay,
  Video,
  LogOut,
} from '@lucide/vue'
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { useAuthStore } from '@/session/auth'
import { useManagerStore } from '@/stores/manager'

const route = useRoute()
const auth = useAuthStore()
const manager = useManagerStore()

const nav = [
  { to: '/posts', label: 'Posts', icon: FileText },
  { to: '/blog', label: 'Blog Poster', icon: MonitorCog },
  { to: '/jobs', label: 'Jobs', icon: CircleGauge },
  { to: '/messaging', label: 'Messaging', icon: MessagesSquare },
  { to: '/timelapse', label: 'Timelapse', icon: Clock3 },
  { to: '/videos', label: 'Videos', icon: Video },
  { to: '/settings', label: 'Settings', icon: Settings },
]

const activeJobs = computed(() => manager.activeJobs + manager.pendingQueue)
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <div class="flex min-h-screen">
      <aside class="sticky top-0 hidden h-screen w-72 overflow-y-auto border-r bg-[#f2f6ea] p-3 lg:block">
        <div class="mb-4 flex items-center gap-2 px-2 py-3">
          <SquarePlay class="h-5 w-5 text-primary" />
          <div>
            <p class="text-sm font-semibold">Herb Hub</p>
            <p class="text-xs text-muted-foreground">Manager Operations</p>
          </div>
        </div>
        <nav class="space-y-1">
          <RouterLink
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            class="flex items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-[#e5efd8]"
            :class="route.path === item.to ? 'bg-[#dceac8] font-semibold' : ''"
          >
            <component :is="item.icon" class="h-4 w-4" />
            {{ item.label }}
            <span
              v-if="item.to === '/jobs' && activeJobs > 0"
              class="ml-auto rounded-full bg-primary px-2 py-0.5 text-xs text-primary-foreground"
            >
              {{ activeJobs }}
            </span>
          </RouterLink>
        </nav>
      </aside>

      <main class="flex-1">
        <header class="sticky top-0 z-20 flex items-center justify-between border-b bg-background/95 p-3 backdrop-blur lg:px-6">
          <div class="flex items-center gap-2 lg:hidden">
            <Sheet>
              <SheetTrigger as-child>
                <Button variant="outline" size="sm">
                  <Menu class="h-4 w-4" />
                  Menu
                </Button>
              </SheetTrigger>
              <SheetContent side="left">
                <SheetHeader>
                  <SheetTitle>Manager Navigation</SheetTitle>
                </SheetHeader>
                <nav class="mt-4 space-y-1">
                  <RouterLink
                    v-for="item in nav"
                    :key="item.to"
                    :to="item.to"
                    class="flex items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-muted"
                  >
                    <component :is="item.icon" class="h-4 w-4" />
                    {{ item.label }}
                  </RouterLink>
                </nav>
              </SheetContent>
            </Sheet>
          </div>

          <div class="text-xs text-muted-foreground">{{ auth.authDisabled ? 'Auth disabled (local)' : auth.account?.username }}</div>
          <Button v-if="!auth.authDisabled" variant="ghost" size="sm" @click="auth.logout">
            <LogOut class="h-4 w-4" />
            Sign out
          </Button>
        </header>

        <section class="p-3 md:p-4 lg:p-6">
          <slot />
        </section>
      </main>
    </div>
  </div>
</template>
