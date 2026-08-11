<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { ApiError, apiFetch } from '@/api/client'
import StatePanel from '@/components/StatePanel.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  AUTOMATIC_PLANTS,
  createAutomaticDraft,
  formatDateTime,
  formatDecision,
  formatDurationSeconds,
  isAutomaticDraftDirty,
  normalizeAutomaticWateringRepresentation,
  requiresEnableConfirmation,
  sampleAgeSeconds,
  shouldResetEnableConfirmation,
  validateAutomaticDraft,
} from '@/lib/automatic-watering'
import type { AutomaticWateringDraft } from '@/lib/automatic-watering'
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
import type {
  AutomaticPlantKey,
  AutomaticWateringRepresentation,
  MessagingOverview,
  MessagingPublishResult,
  MessagingTemplateInfo,
} from '@/types/api'

type TabKey = 'topology' | 'publish' | 'metrics' | 'automatic'

const AUTOMATIC_ENDPOINT = '/api/messaging/automatic-watering'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'topology', label: 'Topology' },
  { key: 'publish', label: 'Publish' },
  { key: 'metrics', label: 'Metrics' },
  { key: 'automatic', label: 'Automatic' },
]

const activeTab = ref<TabKey>('topology')
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const overview = ref<MessagingOverview | null>(null)

const automaticLoading = ref(true)
const automaticRefreshing = ref(false)
const automaticSaving = ref(false)
const automaticError = ref('')
const automaticSuccess = ref('')
const automaticStaleMessage = ref('')
const automaticData = ref<AutomaticWateringRepresentation | null>(null)
const automaticDraft = ref<AutomaticWateringDraft | null>(null)
const automaticValidationErrors = ref<Record<string, string>>({})
const automaticEnableConfirm = ref(false)

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

const automaticValidation = computed(() => {
  if (!automaticDraft.value) return { errors: {} }
  return validateAutomaticDraft(automaticDraft.value)
})

const automaticNeedsEnableConfirmation = computed(() => {
  if (!automaticData.value || !automaticDraft.value) return false
  return requiresEnableConfirmation(automaticData.value.config.enabled, automaticDraft.value.enabled)
})

const automaticDirty = computed(() => {
  if (!automaticData.value || !automaticDraft.value) return false
  return isAutomaticDraftDirty(automaticData.value.config, automaticDraft.value)
})

const automaticCanSave = computed(() => {
  if (!automaticData.value || !automaticDraft.value || automaticSaving.value || automaticLoading.value) return false
  if (!automaticDirty.value) return false
  if (Object.keys(automaticValidation.value.errors).length > 0) return false
  if (automaticNeedsEnableConfirmation.value && !automaticEnableConfirm.value) return false
  return true
})

function fieldError(path: string): string {
  return automaticValidationErrors.value[path] || ''
}

function onAutomaticDraftEdited() {
  const enableTransitionPending = automaticNeedsEnableConfirmation.value
  if (shouldResetEnableConfirmation({ fieldPath: 'draft', isEnableTransitionPending: enableTransitionPending })) {
    automaticEnableConfirm.value = false
  }
  automaticSuccess.value = ''
  automaticStaleMessage.value = ''
  automaticValidationErrors.value = automaticValidation.value.errors
}

function onAutomaticEnableConfirmChanged() {
  automaticSuccess.value = ''
  automaticStaleMessage.value = ''
}

function formatHoursHint(minutesInput: string): string {
  const parsed = Number(minutesInput)
  if (!Number.isFinite(parsed)) return '—'
  return `${(parsed / 60).toFixed(2).replace(/\.00$/, '')}h`
}

function formatHoursValue(hoursInput: string): string {
  const parsed = Number(hoursInput)
  if (!Number.isFinite(parsed)) return '—'
  return `${parsed.toFixed(2).replace(/\.00$/, '')}h`
}

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

async function loadAutomaticWatering() {
  automaticError.value = ''
  try {
    const raw = await apiFetch<unknown>(AUTOMATIC_ENDPOINT)
    const normalized = normalizeAutomaticWateringRepresentation(raw)
    automaticData.value = normalized
    automaticDraft.value = createAutomaticDraft(normalized.config)
    automaticValidationErrors.value = {}
    automaticEnableConfirm.value = false
  } catch (e) {
    automaticError.value = (e as Error).message
    automaticData.value = null
    automaticDraft.value = null
  }
}

async function refreshOverview() {
  refreshing.value = true
  await loadOverview()
  refreshing.value = false
}

async function refreshAutomaticWatering() {
  automaticRefreshing.value = true
  automaticSuccess.value = ''
  automaticStaleMessage.value = ''
  await loadAutomaticWatering()
  automaticRefreshing.value = false
}

async function saveAutomaticWatering() {
  if (!automaticData.value || !automaticDraft.value) return

  automaticError.value = ''
  automaticSuccess.value = ''
  automaticStaleMessage.value = ''
  const validation = validateAutomaticDraft(automaticDraft.value)
  automaticValidationErrors.value = validation.errors

  if (!validation.parsedConfig) {
    automaticError.value = 'Fix validation errors before saving. Server-side validation still applies.'
    automaticEnableConfirm.value = false
    return
  }

  const mustConfirmEnable = requiresEnableConfirmation(automaticData.value.config.enabled, validation.parsedConfig.enabled)
  if (mustConfirmEnable && !automaticEnableConfirm.value) {
    automaticError.value = 'Enable confirmation is required before turning on automatic watering.'
    automaticEnableConfirm.value = false
    return
  }

  const revision = automaticData.value.config_revision
  if (!Number.isFinite(revision) || revision < 1) {
    automaticError.value = 'Cannot save because configuration revision is missing. Refresh and try again.'
    automaticEnableConfirm.value = false
    return
  }

  automaticSaving.value = true
  try {
    const ifMatch = `"${Math.trunc(revision)}"`
    const raw = await apiFetch<unknown>(AUTOMATIC_ENDPOINT, {
      method: 'PUT',
      headers: {
        'If-Match': ifMatch,
      },
      body: {
        config: validation.parsedConfig,
        confirm_enable: mustConfirmEnable ? automaticEnableConfirm.value : false,
      },
    })
    const normalized = normalizeAutomaticWateringRepresentation(raw)
    automaticData.value = normalized
    automaticDraft.value = createAutomaticDraft(normalized.config)
    automaticValidationErrors.value = {}
    automaticSuccess.value = 'Automatic watering configuration saved.'
  } catch (e) {
    if (e instanceof ApiError && e.status === 412) {
      automaticStaleMessage.value = 'Configuration changed on the server (stale revision). Refresh automatic status and review differences before saving again.'
    } else if (e instanceof ApiError && e.status === 428) {
      automaticError.value = 'Server rejected the save because a precondition header is missing. Refresh and retry.'
    } else {
      automaticError.value = (e as Error).message
    }
  } finally {
    automaticSaving.value = false
    automaticEnableConfirm.value = false
  }
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

function automaticServiceStateClass(): string {
  if (!automaticData.value) return 'text-slate-900 bg-slate-50 border-slate-300'
  const status = automaticData.value.status
  if (status.faulted) return 'text-red-900 bg-red-50 border-red-300'
  if (status.running) return 'text-emerald-900 bg-emerald-50 border-emerald-300'
  return 'text-red-900 bg-red-50 border-red-300'
}

function automaticServiceLabel(): string {
  if (!automaticData.value) return 'unknown'
  const status = automaticData.value.status
  if (status.faulted) return 'faulted'
  return status.running ? 'running' : 'stopped'
}

function plantRuntime(plant: AutomaticPlantKey) {
  return automaticData.value?.status.plants[plant]
}

onMounted(async () => {
  loading.value = true
  automaticLoading.value = true
  await Promise.all([loadOverview(), loadAutomaticWatering()])
  loading.value = false
  automaticLoading.value = false
})
</script>

<template>
  <div class="space-y-4">
    <header class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 class="text-lg font-semibold">Messaging</h1>
        <p class="text-sm text-muted-foreground">Curated RabbitMQ setup, template publishing, and automatic watering configuration.</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="outline" :disabled="refreshing" @click="refreshOverview">{{ refreshing ? 'Refreshing…' : 'Refresh overview' }}</Button>
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
              class="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-semibold hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              Open Grafana dashboard
            </a>
          </CardContent>
        </Card>
      </section>

      <section
        v-show="activeTab === 'automatic'"
        :id="panelId('automatic')"
        role="tabpanel"
        :aria-labelledby="tabId('automatic')"
        class="space-y-3"
      >
        <header class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-base font-semibold">Automatic watering</h2>
            <p class="text-sm text-muted-foreground">Configuration and runtime state for fixed plants: basil, chilli, oregano.</p>
          </div>
          <Button variant="outline" :disabled="automaticRefreshing || automaticLoading" @click="refreshAutomaticWatering">
            {{ automaticRefreshing ? 'Refreshing…' : 'Refresh automatic status' }}
          </Button>
        </header>

        <p v-if="automaticSuccess" role="status" aria-live="polite" class="rounded border border-emerald-300 bg-emerald-50 p-2 text-sm text-emerald-900">{{ automaticSuccess }}</p>
        <p v-if="automaticStaleMessage" role="alert" aria-live="assertive" class="rounded border border-amber-300 bg-amber-50 p-2 text-sm text-amber-900">
          {{ automaticStaleMessage }}
          <Button size="sm" variant="outline" class="ml-2" :disabled="automaticRefreshing" @click="refreshAutomaticWatering">Refresh now</Button>
        </p>
        <p v-if="automaticError" role="alert" aria-live="assertive" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">{{ automaticError }}</p>

        <StatePanel v-if="automaticLoading" title="Loading" message="Loading automatic watering status..." kind="loading" />
        <StatePanel v-else-if="!automaticData || !automaticDraft" title="Unavailable" message="Automatic watering data is unavailable." kind="empty" />

        <template v-else>
          <Card>
            <CardContent class="space-y-3 pt-6">
              <div class="grid gap-2 sm:grid-cols-3">
                <p
                  class="rounded border p-2 text-sm"
                  :class="automaticServiceStateClass()"
                >
                  <span class="font-semibold">Service:</span> {{ automaticServiceLabel() }}
                </p>
                <p class="rounded border border-border bg-muted/40 p-2 text-sm">
                  <span class="font-semibold">Next evaluation:</span>
                  {{ formatDateTime(automaticData.status.next_evaluation_at) }}
                </p>
                <p class="rounded border border-border bg-muted/40 p-2 text-sm">
                  <span class="font-semibold">Config revision:</span> {{ automaticData.config_revision || '—' }}
                </p>
              </div>
              <p v-if="automaticData.status.instance_id" class="rounded border border-border bg-muted/40 p-2 text-xs">
                <span class="font-semibold">Instance ID:</span> {{ automaticData.status.instance_id }}
              </p>
              <p v-if="automaticData.status.fault" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">
                <span class="font-semibold">Runtime error:</span> {{ automaticData.status.fault }}
              </p>
              <p class="rounded border border-emerald-300 bg-emerald-50 p-2 text-xs text-emerald-900">
                Automatic publisher behavior: only moisture strictly below threshold publishes <span class="font-semibold">water</span>. Equality, healthy, stale metrics, errors, and cooldown publish nothing. There are no <span class="font-semibold">skip</span> messages. Every water message expires within configured max 5 minutes. Raspberry Pi watering pulse duration is fixed at 15 seconds.
              </p>
              <p v-if="automaticData.config.enabled" class="rounded border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
                Automatic watering is enabled. Configuration edits are allowed and apply to future evaluations only; existing cooldown history remains in effect.
              </p>
              <p v-else-if="automaticDraft.enabled" class="rounded border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
                This unsaved draft enables automatic watering. Automation remains disabled until the configuration is explicitly confirmed and saved.
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Global configuration</CardTitle>
              <CardDescription>Use minutes for interval values (with hours hints). Server accepts integer seconds.</CardDescription>
            </CardHeader>
            <CardContent class="space-y-4">
              <label class="flex cursor-pointer items-center gap-2 text-sm font-medium">
                <input
                  v-model="automaticDraft.enabled"
                  type="checkbox"
                  class="h-4 w-4 cursor-pointer"
                  @change="onAutomaticDraftEdited"
                />
                Enable automatic watering
              </label>

              <div
                v-if="automaticNeedsEnableConfirmation"
                class="space-y-2 rounded border border-red-300 bg-red-50 p-3 text-sm text-red-900"
              >
                <p class="font-semibold">Physical automation warning</p>
                <p>
                  Enabling this feature allows automated watering messages to be sent without manual publish actions when thresholds are crossed.
                </p>
                <label class="flex cursor-pointer items-center gap-2">
                  <input
                    v-model="automaticEnableConfirm"
                    type="checkbox"
                    class="h-4 w-4 cursor-pointer"
                    @change="onAutomaticEnableConfirmChanged"
                  />
                  I explicitly confirm enabling physical automatic watering now.
                </label>
              </div>

              <p v-if="fieldError('plants_metric_label_value_unique')" class="rounded border border-red-300 bg-red-50 p-2 text-xs text-red-700">
                {{ fieldError('plants_metric_label_value_unique') }}
              </p>

              <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                <div class="space-y-1">
                  <label for="automatic-evaluation-interval" class="block cursor-pointer text-xs font-semibold">Evaluation interval (minutes)</label>
                  <Input
                    id="automatic-evaluation-interval"
                    v-model="automaticDraft.evaluation_interval_minutes"
                    inputmode="decimal"
                    :aria-invalid="!!fieldError('evaluation_interval_minutes')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p class="text-xs text-muted-foreground">~ {{ formatHoursHint(automaticDraft.evaluation_interval_minutes) }}</p>
                  <p v-if="fieldError('evaluation_interval_minutes')" class="text-xs text-red-700">{{ fieldError('evaluation_interval_minutes') }}</p>
                </div>

                <div class="space-y-1">
                  <label for="automatic-cooldown" class="block cursor-pointer text-xs font-semibold">Cooldown (hours)</label>
                  <Input
                    id="automatic-cooldown"
                    v-model="automaticDraft.cooldown_hours"
                    inputmode="decimal"
                    :aria-invalid="!!fieldError('cooldown_hours')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p class="text-xs text-muted-foreground">~ {{ formatHoursValue(automaticDraft.cooldown_hours) }}</p>
                  <p v-if="fieldError('cooldown_hours')" class="text-xs text-red-700">{{ fieldError('cooldown_hours') }}</p>
                </div>

                <div class="space-y-1">
                  <label for="automatic-max-age" class="block cursor-pointer text-xs font-semibold">Max metric age (minutes)</label>
                  <Input
                    id="automatic-max-age"
                    v-model="automaticDraft.max_metric_age_minutes"
                    inputmode="decimal"
                    :aria-invalid="!!fieldError('max_metric_age_minutes')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p class="text-xs text-muted-foreground">Must be ≥ evaluation interval.</p>
                  <p v-if="fieldError('max_metric_age_minutes')" class="text-xs text-red-700">{{ fieldError('max_metric_age_minutes') }}</p>
                </div>

                <div class="space-y-1">
                  <label for="automatic-prom-timeout" class="block cursor-pointer text-xs font-semibold">Prometheus timeout (seconds)</label>
                  <Input
                    id="automatic-prom-timeout"
                    v-model="automaticDraft.prometheus_timeout_seconds"
                    inputmode="numeric"
                    :aria-invalid="!!fieldError('prometheus_timeout_seconds')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p v-if="fieldError('prometheus_timeout_seconds')" class="text-xs text-red-700">{{ fieldError('prometheus_timeout_seconds') }}</p>
                </div>

                <div class="space-y-1">
                  <label for="automatic-message-expiry" class="block cursor-pointer text-xs font-semibold">Message expiry (minutes)</label>
                  <Input
                    id="automatic-message-expiry"
                    v-model="automaticDraft.message_expiry_minutes"
                    inputmode="decimal"
                    :aria-invalid="!!fieldError('message_expiry_minutes')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p class="text-xs text-muted-foreground">Hard maximum is 5 minutes.</p>
                  <p v-if="fieldError('message_expiry_minutes')" class="text-xs text-red-700">{{ fieldError('message_expiry_minutes') }}</p>
                </div>

                <div class="space-y-1">
                  <label for="automatic-moisture-metric" class="block cursor-pointer text-xs font-semibold">Moisture metric name</label>
                  <Input
                    id="automatic-moisture-metric"
                    v-model="automaticDraft.moisture_metric"
                    :aria-invalid="!!fieldError('moisture_metric')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p v-if="fieldError('moisture_metric')" class="text-xs text-red-700">{{ fieldError('moisture_metric') }}</p>
                </div>

                <div class="space-y-1 sm:col-span-2 xl:col-span-3">
                  <label for="automatic-plant-label" class="block cursor-pointer text-xs font-semibold">Plant label key</label>
                  <Input
                    id="automatic-plant-label"
                    v-model="automaticDraft.plant_label"
                    :aria-invalid="!!fieldError('plant_label')"
                    @input="onAutomaticDraftEdited"
                  />
                  <p class="text-xs text-muted-foreground">This label key is matched against each plant label value below.</p>
                  <p v-if="fieldError('plant_label')" class="text-xs text-red-700">{{ fieldError('plant_label') }}</p>
                </div>
              </div>

              <div class="flex flex-wrap items-center gap-2">
                <Button :disabled="!automaticCanSave" @click="saveAutomaticWatering">{{ automaticSaving ? 'Saving…' : 'Save automatic config' }}</Button>
                <Button variant="outline" :disabled="automaticRefreshing" @click="refreshAutomaticWatering">Revert from server</Button>
                <p class="text-xs text-muted-foreground">Changes are full replacement saves using revision preconditions.</p>
              </div>
            </CardContent>
          </Card>

          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Card v-for="plant in AUTOMATIC_PLANTS" :key="plant">
              <CardHeader>
                <CardTitle class="capitalize">{{ plant }}</CardTitle>
                <CardDescription>Plant configuration and runtime state</CardDescription>
              </CardHeader>
              <CardContent class="space-y-3">
                <label class="flex cursor-pointer items-center gap-2 text-xs font-semibold">
                  <input
                    v-model="automaticDraft.plants[plant].enabled"
                    type="checkbox"
                    class="h-4 w-4 cursor-pointer"
                    @change="onAutomaticDraftEdited"
                  />
                  Enabled
                </label>

                <div class="space-y-1">
                  <label :for="`automatic-${plant}-threshold`" class="block cursor-pointer text-xs font-semibold">Threshold (%)</label>
                  <Input
                    :id="`automatic-${plant}-threshold`"
                    v-model="automaticDraft.plants[plant].threshold_percent"
                    inputmode="decimal"
                    :aria-invalid="!!fieldError(`plants.${plant}.threshold_percent`)"
                    @input="onAutomaticDraftEdited"
                  />
                  <p v-if="fieldError(`plants.${plant}.threshold_percent`)" class="text-xs text-red-700">{{ fieldError(`plants.${plant}.threshold_percent`) }}</p>
                </div>

                <div class="space-y-1">
                  <label :for="`automatic-${plant}-label-value`" class="block cursor-pointer text-xs font-semibold">Metric label value</label>
                  <Input
                    :id="`automatic-${plant}-label-value`"
                    v-model="automaticDraft.plants[plant].metric_label_value"
                    :aria-invalid="!!fieldError(`plants.${plant}.metric_label_value`)"
                    @input="onAutomaticDraftEdited"
                  />
                  <p v-if="fieldError(`plants.${plant}.metric_label_value`)" class="text-xs text-red-700">{{ fieldError(`plants.${plant}.metric_label_value`) }}</p>
                </div>

                <div class="grid grid-cols-1 gap-1 rounded border border-border bg-muted/40 p-2 text-xs">
                  <p><span class="font-semibold">Last measurement:</span> {{ plantRuntime(plant)?.last_value ?? '—' }}</p>
                  <p><span class="font-semibold">Sample age:</span> {{ formatDurationSeconds(sampleAgeSeconds(plantRuntime(plant)?.last_evaluated_at, plantRuntime(plant)?.last_sample_at)) }}</p>
                  <p><span class="font-semibold">Sample time:</span> {{ formatDateTime(plantRuntime(plant)?.last_sample_at) }}</p>
                  <p><span class="font-semibold">Last evaluation:</span> {{ formatDateTime(plantRuntime(plant)?.last_evaluated_at) }}</p>
                  <p><span class="font-semibold">Decision:</span> {{ formatDecision(plantRuntime(plant)?.last_decision) }}</p>
                  <p><span class="font-semibold">Cooldown until:</span> {{ formatDateTime(plantRuntime(plant)?.cooldown_until) }}</p>
                  <p><span class="font-semibold">Last message ID:</span> {{ plantRuntime(plant)?.last_message_id || '—' }}</p>
                  <p v-if="plantRuntime(plant)?.last_error" class="text-red-700"><span class="font-semibold">Error:</span> {{ plantRuntime(plant)?.last_error }}</p>
                </div>
              </CardContent>
            </Card>
          </div>
        </template>
      </section>
    </template>
  </div>
</template>
