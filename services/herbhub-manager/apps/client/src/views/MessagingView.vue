<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { apiFetch } from '@/api/client'
import StatePanel from '@/components/StatePanel.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  canProvisionCatalogue,
  derivePublishRoute,
  formatTemplatePayload,
  normalizeMessagingOverview,
  normalizeMessagingPublishResult,
  parsePayloadObject,
  getPublishReadinessBlockReason,
  resetPublishConfirmationState,
  requiresDangerConfirmation,
  validateWateringPayload,
} from '@/lib/messaging'
import type { MessagingOverview, MessagingPublishResult, MessagingTemplateInfo } from '@/types/api'

type TabKey = 'topology' | 'publish' | 'metrics'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'topology', label: 'Topology' },
  { key: 'publish', label: 'Publish' },
  { key: 'metrics', label: 'Metrics' },
]

const activeTab = ref<TabKey>('topology')
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const overview = ref<MessagingOverview | null>(null)

const provisioningCatalogue = ref('')
const provisionConfirmFor = ref('')
const provisionMessage = ref('')
const provisionError = ref('')

const selectedTemplateId = ref('')
const payloadText = ref('')
const selectedRoutingKey = ref('')
const publishConfirmed = ref(false)
const dangerConfirmed = ref(false)
const publishing = ref(false)
const publishError = ref('')
const publishResult = ref<MessagingPublishResult | null>(null)
const tabRefs = ref<Partial<Record<TabKey, HTMLButtonElement | null>>>({})

const selectedTemplate = computed<MessagingTemplateInfo | undefined>(() => {
  if (!overview.value) return undefined
  return overview.value.templates.find((t) => t.id === selectedTemplateId.value)
})

const routingOptions = computed(() => selectedTemplate.value?.allowed_routing_keys ?? [])
const showRoutingSelect = computed(() => routingOptions.value.length > 0)

const parsedPayload = computed(() => parsePayloadObject(payloadText.value))
const wateringErrors = computed(() => {
  if (!selectedTemplate.value || !parsedPayload.value.value) return []
  return validateWateringPayload(selectedTemplate.value.id, parsedPayload.value.value)
})
const resolvedRoute = computed(() => {
  if (!selectedTemplate.value || !parsedPayload.value.value) return ''
  return derivePublishRoute(selectedTemplate.value.id, parsedPayload.value.value, selectedRoutingKey.value)
})

const confirmationNeeded = computed(() => requiresDangerConfirmation(selectedTemplate.value))
const publishReadinessReason = computed(() =>
  getPublishReadinessBlockReason({
    overview: overview.value,
    template: selectedTemplate.value,
  }),
)

const canSubmitPublish = computed(() => {
  if (!selectedTemplate.value || publishing.value) return false
  if (publishReadinessReason.value) return false
  if (parsedPayload.value.error || !parsedPayload.value.value) return false
  if (wateringErrors.value.length > 0) return false
  if (!publishConfirmed.value) return false
  if (confirmationNeeded.value && (!publishConfirmed.value || !dangerConfirmed.value)) return false
  return true
})

async function loadOverview() {
  error.value = ''
  try {
    const raw = await apiFetch<unknown>('/api/messaging/overview')
    overview.value = normalizeMessagingOverview(raw)
    if (!selectedTemplateId.value && overview.value.templates.length > 0) {
      const first = overview.value.templates[0]
      if (first) {
        selectTemplate(first.id)
      }
    }
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function refreshOverview() {
  refreshing.value = true
  await loadOverview()
  refreshing.value = false
}

function selectTemplate(templateId: string) {
  selectedTemplateId.value = templateId
  publishResult.value = null
  publishError.value = ''
  publishConfirmed.value = false
  dangerConfirmed.value = false

  const template = overview.value?.templates.find((t) => t.id === templateId)
  if (!template) {
    payloadText.value = '{}'
    selectedRoutingKey.value = ''
    return
  }

  payloadText.value = formatTemplatePayload(template.default_payload)
  selectedRoutingKey.value = template.default_routing_key ?? ''
}

function tabId(key: TabKey) {
  return `messaging-tab-${key}`
}

function panelId(key: TabKey) {
  return `messaging-panel-${key}`
}

function setTabRef(key: TabKey, element: HTMLButtonElement | null) {
  tabRefs.value[key] = element
}

async function setActiveTabAndFocus(key: TabKey) {
  activeTab.value = key
  await nextTick()
  const target = tabRefs.value[key] ?? document.getElementById(tabId(key))
  if (target instanceof HTMLButtonElement) {
    target.focus()
  }
}

function onTabsKeydown(event: KeyboardEvent) {
  const currentIndex = tabs.findIndex((tab) => tab.key === activeTab.value)
  if (currentIndex < 0) return

  if (event.key === 'ArrowRight') {
    event.preventDefault()
    const next = (currentIndex + 1) % tabs.length
    const nextTab = tabs[next]
    if (nextTab) {
      void setActiveTabAndFocus(nextTab.key)
    }
    return
  }

  if (event.key === 'ArrowLeft') {
    event.preventDefault()
    const next = (currentIndex - 1 + tabs.length) % tabs.length
    const nextTab = tabs[next]
    if (nextTab) {
      void setActiveTabAndFocus(nextTab.key)
    }
    return
  }

  if (event.key === 'Home') {
    event.preventDefault()
    const firstTab = tabs[0]
    if (firstTab) {
      void setActiveTabAndFocus(firstTab.key)
    }
    return
  }

  if (event.key === 'End') {
    event.preventDefault()
    const lastTab = tabs[tabs.length - 1]
    if (lastTab) {
      void setActiveTabAndFocus(lastTab.key)
    }
  }
}

async function provisionCatalogue(catalogueId: string) {
  provisionError.value = ''
  provisionMessage.value = ''
  provisioningCatalogue.value = catalogueId

  try {
    await apiFetch(`/api/messaging/catalogues/${catalogueId}/provision`, { method: 'POST' })
    provisionMessage.value = `Provisioned ${catalogueId}.`
    provisionConfirmFor.value = ''
  } catch (e) {
    provisionError.value = (e as Error).message
  } finally {
    provisioningCatalogue.value = ''
    await loadOverview()
  }
}

async function publishTemplate() {
  if (!selectedTemplate.value || !parsedPayload.value.value) return

  publishError.value = ''
  publishResult.value = null
  publishing.value = true
  try {
    const raw = await apiFetch<unknown>(`/api/messaging/templates/${selectedTemplate.value.id}/publish`, {
      method: 'POST',
      body: {
        payload: parsedPayload.value.value,
        routing_key: resolvedRoute.value || undefined,
        confirmed: confirmationNeeded.value ? publishConfirmed.value && dangerConfirmed.value : publishConfirmed.value,
      },
    })
    publishResult.value = normalizeMessagingPublishResult(raw)
  } catch (e) {
    publishError.value = (e as Error).message
  } finally {
    const reset = resetPublishConfirmationState()
    publishConfirmed.value = reset.publishConfirmed
    dangerConfirmed.value = reset.dangerConfirmed
    publishing.value = false
  }
}

function badgeVariantForState(state: string): 'success' | 'warning' | 'danger' | 'secondary' {
  if (state === 'ready') return 'success'
  if (state === 'missing') return 'warning'
  if (state === 'drifted' || state === 'unavailable') return 'danger'
  return 'secondary'
}

onMounted(async () => {
  loading.value = true
  await loadOverview()
  loading.value = false
})
</script>

<template>
  <div class="space-y-4">
    <header class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold">Messaging</h1>
        <p class="text-sm text-muted-foreground">Curated RabbitMQ setup, template publishing, and Prometheus summary.</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="outline" :disabled="refreshing" @click="refreshOverview">{{ refreshing ? 'Refreshing…' : 'Refresh' }}</Button>
      </div>
    </header>

    <StatePanel v-if="loading" title="Loading" message="Loading messaging overview..." kind="loading" />
    <StatePanel v-else-if="error" title="Error" :message="error" kind="error" />
    <StatePanel v-else-if="!overview" title="Unavailable" message="Messaging overview is unavailable." kind="empty" />

    <template v-else>
      <Card>
        <CardContent class="pt-6">
          <div class="grid gap-2 text-sm sm:grid-cols-3">
            <p><span class="font-semibold">Messaging:</span> {{ overview.enabled ? 'Enabled' : 'Disabled' }}</p>
            <p><span class="font-semibold">Broker status:</span> {{ overview.broker_status }}</p>
            <p v-if="overview.error" class="text-red-700"><span class="font-semibold">Error:</span> {{ overview.error }}</p>
          </div>
        </CardContent>
      </Card>

      <div role="tablist" aria-label="Messaging sections" class="flex flex-wrap gap-2" @keydown="onTabsKeydown">
        <button
          v-for="tab in tabs"
          :id="tabId(tab.key)"
          :key="tab.key"
          :ref="(el) => setTabRef(tab.key, el as HTMLButtonElement | null)"
          role="tab"
          :aria-selected="activeTab === tab.key"
          :aria-controls="panelId(tab.key)"
          :tabindex="activeTab === tab.key ? 0 : -1"
          class="inline-flex h-9 cursor-pointer items-center rounded-md border px-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="activeTab === tab.key ? 'bg-[#dceac8] border-[#c5daa5]' : 'bg-background hover:bg-muted'"
          @click="void setActiveTabAndFocus(tab.key)"
        >
          {{ tab.label }}
        </button>
      </div>

      <section
        v-show="activeTab === 'topology'"
        :id="panelId('topology')"
        role="tabpanel"
        :aria-labelledby="tabId('topology')"
        class="space-y-3"
      >
        <p v-if="provisionMessage" role="status" aria-live="polite" class="rounded border border-emerald-300 bg-emerald-50 p-2 text-sm text-emerald-900">{{ provisionMessage }}</p>
        <p v-if="provisionError" role="alert" aria-live="assertive" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ provisionError }}</p>

        <div class="grid gap-3 xl:grid-cols-2">
          <Card v-for="catalogue in overview.catalogues" :key="catalogue.id">
            <CardHeader>
              <div class="flex items-start justify-between gap-2">
                <div>
                  <CardTitle>{{ catalogue.name }}</CardTitle>
                  <CardDescription>{{ catalogue.id }}</CardDescription>
                </div>
                <Badge :variant="badgeVariantForState(catalogue.state)">{{ catalogue.state }}</Badge>
              </div>
            </CardHeader>
            <CardContent class="space-y-3">
              <p v-if="catalogue.error" class="text-sm text-red-700">{{ catalogue.error }}</p>
              <p v-if="catalogue.state === 'drifted'" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-900">
                Drift detected. Manual resolution is required before provisioning.
              </p>

              <div class="grid gap-2 md:grid-cols-2">
                <Card v-for="(q, queueName) in catalogue.queues" :key="queueName">
                  <CardHeader class="pb-2">
                    <CardTitle class="text-sm">{{ queueName }}</CardTitle>
                  </CardHeader>
                  <CardContent class="grid grid-cols-3 gap-2 text-xs">
                    <p><span class="font-semibold">Ready:</span> {{ q.ready }}</p>
                    <p><span class="font-semibold">Unacked:</span> {{ q.unacked }}</p>
                    <p><span class="font-semibold">Consumers:</span> {{ q.consumers }}</p>
                  </CardContent>
                </Card>
              </div>

              <div v-if="catalogue.drift && catalogue.drift.length > 0" class="space-y-2">
                <p class="text-xs font-semibold text-muted-foreground">Drift details</p>
                <ul class="space-y-1 text-xs">
                  <li v-for="(detail, idx) in catalogue.drift" :key="`${catalogue.id}-drift-${idx}`" class="rounded border p-2">
                    <p class="font-semibold">{{ detail.resource }} · {{ detail.field }}</p>
                    <p>Expected: {{ detail.expected }}</p>
                    <p>Actual: {{ detail.actual }}</p>
                  </li>
                </ul>
              </div>

              <div class="space-y-2">
                <p v-if="provisionConfirmFor !== catalogue.id" class="text-xs text-muted-foreground">
                  Provision creates missing curated resources and bindings for {{ catalogue.name }}. It never deletes resources.
                </p>
                <div v-if="provisionConfirmFor === catalogue.id" class="space-y-2 rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-900">
                  <p>
                    Confirm provisioning <span class="font-semibold">{{ catalogue.name }}</span>. This may create exchanges, queues, and bindings, but will never delete resources.
                  </p>
                  <div class="flex flex-wrap gap-2">
                    <Button size="sm" :disabled="provisioningCatalogue === catalogue.id" @click="provisionCatalogue(catalogue.id)">Confirm provision</Button>
                    <Button size="sm" variant="ghost" @click="provisionConfirmFor = ''">Cancel</Button>
                  </div>
                </div>
                <Button
                  v-else
                  size="sm"
                  variant="outline"
                  :disabled="!canProvisionCatalogue(catalogue.state) || provisioningCatalogue === catalogue.id"
                  @click="provisionConfirmFor = catalogue.id"
                >
                  Provision
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </section>

      <section
        v-show="activeTab === 'publish'"
        :id="panelId('publish')"
        role="tabpanel"
        :aria-labelledby="tabId('publish')"
        class="grid gap-3 xl:grid-cols-[380px_1fr]"
      >
        <Card>
          <CardHeader>
            <CardTitle>Template</CardTitle>
            <CardDescription>Use curated payload templates and fixed server-side routes only.</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            <div>
              <label for="messaging-template" class="mb-1 block cursor-pointer text-xs font-semibold">Template</label>
              <Select id="messaging-template" :model-value="selectedTemplateId" @update:model-value="selectTemplate">
                <option value="" disabled>Select template</option>
                <option v-for="template in overview.templates" :key="template.id" :value="template.id">
                  {{ template.name }}
                </option>
              </Select>
            </div>

            <div v-if="selectedTemplate" class="rounded border bg-muted/40 p-2 text-xs">
              <p class="font-semibold">{{ selectedTemplate.description }}</p>
              <p>Catalogue: {{ selectedTemplate.catalogue_id }}</p>
              <p>Exchange: fixed by server-side template configuration</p>
              <p>Routing key: {{ resolvedRoute || 'derived server-side' }}</p>
              <p>No automatic retries are performed by this UI.</p>
            </div>

            <div v-if="showRoutingSelect">
              <label for="messaging-routing" class="mb-1 block cursor-pointer text-xs font-semibold">Routing key</label>
              <Select id="messaging-routing" v-model="selectedRoutingKey">
                <option v-for="route in routingOptions" :key="route" :value="route">{{ route }}</option>
              </Select>
            </div>

            <div class="space-y-2">
              <label class="flex cursor-pointer items-center gap-2 text-xs">
                <input v-model="publishConfirmed" type="checkbox" class="h-4 w-4 cursor-pointer" />
                I understand this publishes directly to curated RabbitMQ resources.
              </label>
              <div v-if="confirmationNeeded" class="rounded border border-red-300 bg-red-50 p-2 text-xs text-red-800">
                <p class="font-semibold">Danger: physical watering command</p>
                <p>"Water plant" controls real hardware. Physical water messages expire after 5 minutes, and every publish requires fresh confirmation.</p>
                <label class="mt-2 flex cursor-pointer items-center gap-2">
                  <input v-model="dangerConfirmed" type="checkbox" class="h-4 w-4 cursor-pointer" />
                  I confirm this watering command should be sent now.
                </label>
              </div>
            </div>

            <p v-if="publishReadinessReason" role="status" aria-live="polite" class="rounded border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
              {{ publishReadinessReason }}
            </p>
            <p v-if="publishError" role="alert" aria-live="assertive" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ publishError }}</p>
            <p v-if="publishResult" role="status" aria-live="polite" class="rounded border border-emerald-300 bg-emerald-50 p-2 text-sm text-emerald-900">
              Published message {{ publishResult.message_id }} via {{ publishResult.exchange }} → {{ publishResult.routing_key }}.
            </p>
            <Button :disabled="!canSubmitPublish" @click="publishTemplate">{{ publishing ? 'Publishing…' : 'Publish' }}</Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Payload JSON</CardTitle>
            <CardDescription>Payload must be a JSON object. Plant health template accepts generic object JSON only.</CardDescription>
          </CardHeader>
          <CardContent class="space-y-2">
            <label for="messaging-payload" class="sr-only">Payload JSON</label>
            <Textarea id="messaging-payload" v-model="payloadText" aria-label="Payload JSON" :rows="22" class="font-mono" spellcheck="false" />
            <p v-if="parsedPayload.error" role="alert" aria-live="assertive" class="text-sm text-red-700">{{ parsedPayload.error }}</p>
            <ul v-else-if="wateringErrors.length > 0" role="alert" aria-live="assertive" class="list-disc space-y-1 pl-5 text-sm text-red-700">
              <li v-for="issue in wateringErrors" :key="issue">{{ issue }}</li>
            </ul>
          </CardContent>
        </Card>
      </section>

      <section
        v-show="activeTab === 'metrics'"
        :id="panelId('metrics')"
        role="tabpanel"
        :aria-labelledby="tabId('metrics')"
        class="space-y-3"
      >
        <Card v-if="overview.prometheus?.available === false">
          <CardContent class="pt-6 text-sm text-muted-foreground">
            Prometheus metrics are unavailable ({{ overview.prometheus.error || 'unknown reason' }}). Topology data above is still available.
          </CardContent>
        </Card>

        <div v-if="overview.prometheus?.queues" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <Card v-for="(q, queueName) in overview.prometheus.queues" :key="queueName">
            <CardHeader>
              <CardTitle class="text-base">{{ queueName }}</CardTitle>
              <CardDescription>Prometheus summary</CardDescription>
            </CardHeader>
            <CardContent class="grid grid-cols-3 gap-2 text-sm">
              <div>
                <p class="text-xs text-muted-foreground">Ready</p>
                <p class="font-semibold">{{ q.ready }}</p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">Unacked</p>
                <p class="font-semibold">{{ q.unacked }}</p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">Consumers</p>
                <p class="font-semibold">{{ q.consumers }}</p>
              </div>
            </CardContent>
          </Card>
        </div>

        <Card v-if="overview.grafana_url">
          <CardHeader>
            <CardTitle>Grafana</CardTitle>
            <CardDescription>Open the curated external dashboard in a new tab.</CardDescription>
          </CardHeader>
          <CardContent>
            <a
              :href="overview.grafana_url"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex h-9 items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-semibold hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              Open Grafana dashboard
            </a>
          </CardContent>
        </Card>
      </section>
    </template>
  </div>
</template>
