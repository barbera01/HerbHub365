import type {
  MessagingCatalogueStatus,
  MessagingDriftDetail,
  MessagingOverview,
  MessagingPrometheusSummary,
  MessagingPublishResult,
  MessagingQueueCounters,
  MessagingTemplateInfo,
} from '@/types/api'

type Dict = Record<string, unknown>

function asObject(value: unknown): Dict {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Dict) : {}
}

function asString(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function asBoolean(value: unknown, fallback = false): boolean {
  return typeof value === 'boolean' ? value : fallback
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

function asQueueCounters(value: unknown): MessagingQueueCounters {
  const obj = asObject(value)
  return {
    ready: asNumber(obj.ready),
    unacked: asNumber(obj.unacked),
    consumers: asNumber(obj.consumers),
  }
}

function normalizeDrift(value: unknown): MessagingDriftDetail[] {
  if (!Array.isArray(value)) return []
  return value.map((item) => {
    const obj = asObject(item)
    return {
      resource: asString(obj.resource),
      field: asString(obj.field),
      expected: asString(obj.expected),
      actual: asString(obj.actual),
    }
  })
}

function normalizeCatalogue(value: unknown): MessagingCatalogueStatus {
  const obj = asObject(value)
  const queuesRaw = asObject(obj.queues)
  const queues: Record<string, MessagingQueueCounters> = {}
  for (const [name, counter] of Object.entries(queuesRaw)) {
    queues[name] = asQueueCounters(counter)
  }

  const state = asString(obj.state, 'unavailable')
  const allowedStates = new Set(['ready', 'missing', 'drifted', 'unavailable', 'disabled'])
  return {
    id: asString(obj.id),
    name: asString(obj.name),
    state: (allowedStates.has(state) ? state : 'unavailable') as MessagingCatalogueStatus['state'],
    queues,
    drift: normalizeDrift(obj.drift),
    error: asString(obj.error),
  }
}

function normalizeTemplate(value: unknown): MessagingTemplateInfo {
  const obj = asObject(value)
  return {
    id: asString(obj.id),
    name: asString(obj.name),
    catalogue_id: asString(obj.catalogue_id),
    description: asString(obj.description),
    requires_confirmation: asBoolean(obj.requires_confirmation),
    allowed_routing_keys: asStringArray(obj.allowed_routing_keys),
    default_routing_key: asString(obj.default_routing_key),
    default_payload: asObject(obj.default_payload),
  }
}

function normalizePrometheus(value: unknown): MessagingPrometheusSummary | undefined {
  if (!value || typeof value !== 'object') return undefined
  const obj = asObject(value)
  const queuesRaw = asObject(obj.queues)
  const queues: Record<string, MessagingQueueCounters> = {}
  for (const [name, counter] of Object.entries(queuesRaw)) {
    queues[name] = asQueueCounters(counter)
  }
  return {
    available: asBoolean(obj.available),
    queues,
    error: asString(obj.error),
  }
}

export function normalizeMessagingOverview(raw: unknown): MessagingOverview {
  const obj = asObject(raw)

  const brokerStatus = asString(obj.broker_status) || asString(obj.brokerStatus, 'disabled')
  const grafanaURL = asString(obj.grafana_url) || asString(obj.grafanaUrl)

  const catalogues = Array.isArray(obj.catalogues) ? obj.catalogues.map(normalizeCatalogue) : []
  const templates = Array.isArray(obj.templates) ? obj.templates.map(normalizeTemplate) : []

  return {
    enabled: asBoolean(obj.enabled),
    broker_status: brokerStatus,
    grafana_url: grafanaURL || undefined,
    catalogues,
    templates,
    prometheus: normalizePrometheus(obj.prometheus),
    error: asString(obj.error) || undefined,
  }
}

export function normalizeMessagingPublishResult(raw: unknown): MessagingPublishResult {
  const obj = asObject(raw)
  return {
    message_id: asString(obj.message_id),
    exchange: asString(obj.exchange),
    routing_key: asString(obj.routing_key),
    routed: asBoolean(obj.routed),
  }
}

export function formatTemplatePayload(payload: Record<string, unknown>): string {
  return JSON.stringify(payload, null, 2)
}

export function parsePayloadObject(input: string): { value?: Record<string, unknown>; error?: string } {
  try {
    const parsed = JSON.parse(input) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { error: 'Payload must be a JSON object.' }
    }
    return { value: parsed as Record<string, unknown> }
  } catch {
    return { error: 'Payload is not valid JSON.' }
  }
}

const VALID_WATERING_PLANTS = new Set(['basil', 'chilli', 'oregano'])
const VALID_WATERING_ACTIONS = new Set(['water', 'skip'])

export function derivePublishRoute(templateId: string, payload: Record<string, unknown>, fallback = ''): string {
  if (templateId.startsWith('watering-')) {
    const plant = typeof payload.plant === 'string' ? payload.plant.trim().toLowerCase() : ''
    return plant ? `watering.${plant}` : ''
  }
  if (templateId === 'plant-health-json') {
    return fallback || 'plant.health.left'
  }
  return fallback
}

export function validateWateringPayload(templateId: string, payload: Record<string, unknown>): string[] {
  if (!templateId.startsWith('watering-')) return []
  const errors: string[] = []

  const plant = typeof payload.plant === 'string' ? payload.plant.trim().toLowerCase() : ''
  if (!VALID_WATERING_PLANTS.has(plant)) {
    errors.push('Plant must be one of: basil, chilli, oregano.')
  }

  const action = typeof payload.action === 'string' ? payload.action.trim().toLowerCase() : ''
  if (!VALID_WATERING_ACTIONS.has(action)) {
    errors.push('Action must be one of: water, skip.')
  }

  const expectedAction = templateId === 'watering-water' ? 'water' : templateId === 'watering-skip' ? 'skip' : ''
  if (expectedAction && action !== expectedAction) {
    errors.push(`Action must be ${expectedAction} for this template.`)
  }

  const value = payload.value
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0 || value > 100) {
    errors.push('Value must be a finite number in range [0, 100].')
  }

  return errors
}

export function canProvisionCatalogue(state: MessagingCatalogueStatus['state']): boolean {
  return state === 'missing'
}

export function requiresDangerConfirmation(template: MessagingTemplateInfo | undefined): boolean {
  return template?.id === 'watering-water' && !!template.requires_confirmation
}

export function getPublishReadinessBlockReason(args: {
  overview: MessagingOverview | null
  template: MessagingTemplateInfo | undefined
}): string {
  const { overview, template } = args
  if (!overview) return 'Messaging overview is not loaded.'
  if (!overview.enabled) return 'Publishing is disabled because messaging is disabled.'
  if (overview.broker_status !== 'ok') return `Publishing is disabled because broker status is ${overview.broker_status}.`
  if (!template) return 'Select a template to publish.'

  const catalogue = overview.catalogues.find((item) => item.id === template.catalogue_id)
  if (!catalogue) return 'Publishing is disabled because template catalogue state is unavailable.'
  if (catalogue.state !== 'ready') {
    return `Publishing is disabled because catalogue ${catalogue.name || catalogue.id} is ${catalogue.state}.`
  }

  if (template.id === 'watering-water') {
    const wateringQueue = catalogue.queues?.['watering.queue']
    const consumers = wateringQueue?.consumers ?? 0
    if (consumers === 0) {
      return 'Publishing is blocked: no active watering consumer.'
    }
    if (consumers !== 1) {
      return `Publishing is blocked: expected exactly one active watering consumer, but found ${consumers}.`
    }
  }

  return ''
}

export function resetPublishConfirmationState(): { publishConfirmed: boolean; dangerConfirmed: boolean } {
  return { publishConfirmed: false, dangerConfirmed: false }
}
