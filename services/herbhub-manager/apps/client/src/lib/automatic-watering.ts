import type {
  AutomaticPlantKey,
  AutomaticWateringConfig,
  AutomaticWateringPlantConfig,
  AutomaticWateringPlantRuntime,
  AutomaticWateringRepresentation,
  AutomaticWateringStatus,
} from '@/types/api'

type Dict = Record<string, unknown>

export const AUTOMATIC_PLANTS: AutomaticPlantKey[] = ['basil', 'chilli', 'oregano']

const METRIC_IDENTIFIER = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/
const LABEL_KEY_IDENTIFIER = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/
const SAFE_LABEL_VALUE = /^[a-zA-Z0-9][a-zA-Z0-9_.:/-]{0,63}$/

const BOUNDS = {
  evaluationIntervalSeconds: { min: 60, max: 3600 },
  cooldownSeconds: { min: 3600, max: 604800 },
  maxMetricAgeSeconds: { min: 60, max: 3600 },
  prometheusTimeoutSeconds: { min: 1, max: 30 },
  messageExpirySeconds: { min: 30, max: 300 },
  thresholdPercent: { min: 5, max: 80 },
} as const

export type AutomaticWateringDraft = {
  enabled: boolean
  evaluation_interval_minutes: string
  cooldown_hours: string
  max_metric_age_minutes: string
  prometheus_timeout_seconds: string
  message_expiry_minutes: string
  moisture_metric: string
  plant_label: string
  plants: Record<AutomaticPlantKey, { enabled: boolean; threshold_percent: string; metric_label_value: string }>
}

export type AutomaticDraftValidationResult = {
  errors: Record<string, string>
  parsedConfig?: AutomaticWateringConfig
}

export function shouldResetEnableConfirmation(args: {
  fieldPath: string
  isEnableTransitionPending: boolean
}): boolean {
  const { fieldPath, isEnableTransitionPending } = args
  if (!isEnableTransitionPending) return false
  return fieldPath !== 'enable_confirmation'
}

function asObject(value: unknown): Dict {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Dict) : {}
}

function asString(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function asBoolean(value: unknown, fallback = false): boolean {
  return typeof value === 'boolean' ? value : fallback
}

function asOptionalFiniteNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function asFiniteNumber(value: unknown, fallback = 0): number {
  return asOptionalFiniteNumber(value) ?? fallback
}

export function sanitizeTextForUi(value: string, maxLength = 240): string {
  return value
    .replace(/[\x00-\x1F\x7F]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, maxLength)
}

function normalizePlantConfig(value: unknown): AutomaticWateringPlantConfig {
  const obj = asObject(value)
  return {
    enabled: asBoolean(obj.enabled),
    threshold_percent: asFiniteNumber(obj.threshold_percent),
    metric_label_value: asString(obj.metric_label_value),
  }
}

function normalizePlantRuntime(value: unknown): AutomaticWateringPlantRuntime {
  const obj = asObject(value)
  return {
    last_evaluated_at: asString(obj.last_evaluated_at) || undefined,
    last_sample_at: asString(obj.last_sample_at) || undefined,
    last_value: asOptionalFiniteNumber(obj.last_value),
    last_decision: asString(obj.last_decision) || undefined,
    last_error: sanitizeTextForUi(asString(obj.last_error)) || undefined,
    cooldown_until: asString(obj.cooldown_until) || undefined,
    last_message_id: asString(obj.last_message_id) || undefined,
  }
}

export function normalizeAutomaticWateringRepresentation(raw: unknown): AutomaticWateringRepresentation {
  const obj = asObject(raw)
  const configObj = asObject(obj.config)
  const runtimeObj = asObject(obj.runtime_status)
  const stateObj = asObject(obj.state)

  const plantsConfig = {} as Record<AutomaticPlantKey, AutomaticWateringPlantConfig>
  const plantsRuntime = {} as Record<AutomaticPlantKey, AutomaticWateringPlantRuntime>

  for (const plant of AUTOMATIC_PLANTS) {
    plantsConfig[plant] = normalizePlantConfig(asObject(configObj.plants)[plant])
    plantsRuntime[plant] = normalizePlantRuntime(stateObj[plant])
  }

  const status: AutomaticWateringStatus = {
    running: asBoolean(runtimeObj.running),
    faulted: asBoolean(runtimeObj.faulted),
    fault: sanitizeTextForUi(asString(runtimeObj.fault)) || undefined,
    next_evaluation_at: asString(runtimeObj.next_evaluation_at) || undefined,
    instance_id: asString(runtimeObj.instance_id) || undefined,
    plants: plantsRuntime,
  }

  return {
    config_revision: asFiniteNumber(obj.config_revision),
    config: {
      enabled: asBoolean(configObj.enabled),
      evaluation_interval_seconds: asFiniteNumber(configObj.evaluation_interval_seconds),
      cooldown_seconds: asFiniteNumber(configObj.cooldown_seconds),
      max_metric_age_seconds: asFiniteNumber(configObj.max_metric_age_seconds),
      prometheus_timeout_seconds: asFiniteNumber(configObj.prometheus_timeout_seconds),
      message_expiry_seconds: asFiniteNumber(configObj.message_expiry_seconds),
      moisture_metric: asString(configObj.moisture_metric),
      plant_label: asString(configObj.plant_label),
      plants: plantsConfig,
    },
    status,
  }
}

export function minutesInputToSeconds(value: string): number | undefined {
  const parsed = parseFiniteNumber(value)
  if (parsed === undefined) return undefined
  const seconds = Math.round(parsed * 60)
  return Number.isFinite(seconds) ? seconds : undefined
}

export function hoursInputToSeconds(value: string): number | undefined {
  const parsed = parseFiniteNumber(value)
  if (parsed === undefined) return undefined
  const seconds = Math.round(parsed * 3600)
  return Number.isFinite(seconds) ? seconds : undefined
}

export function secondsToMinutesInput(seconds: number): string {
  if (!Number.isFinite(seconds)) return ''
  return numberToInput(seconds / 60)
}

export function secondsToHoursInput(seconds: number): string {
  if (!Number.isFinite(seconds)) return ''
  return numberToInput(seconds / 3600)
}

export function numberToInput(value: number): string {
  if (!Number.isFinite(value)) return ''
  return value.toFixed(3).replace(/\.0+$/, '').replace(/(\.\d*?)0+$/, '$1')
}

function parseFiniteNumber(value: string): number | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

export function createAutomaticDraft(config: AutomaticWateringConfig): AutomaticWateringDraft {
  const plants = {} as AutomaticWateringDraft['plants']
  for (const plant of AUTOMATIC_PLANTS) {
    const source = config.plants[plant]
    plants[plant] = {
      enabled: source.enabled,
      threshold_percent: numberToInput(source.threshold_percent),
      metric_label_value: source.metric_label_value,
    }
  }
  return {
    enabled: config.enabled,
    evaluation_interval_minutes: secondsToMinutesInput(config.evaluation_interval_seconds),
    cooldown_hours: secondsToHoursInput(config.cooldown_seconds),
    max_metric_age_minutes: secondsToMinutesInput(config.max_metric_age_seconds),
    prometheus_timeout_seconds: numberToInput(config.prometheus_timeout_seconds),
    message_expiry_minutes: secondsToMinutesInput(config.message_expiry_seconds),
    moisture_metric: config.moisture_metric,
    plant_label: config.plant_label,
    plants,
  }
}

function enforceBounds(name: string, value: number, min: number, max: number, errors: Record<string, string>, path: string) {
  if (value < min || value > max) {
    errors[path] = `${name} must be between ${min} and ${max}.`
  }
}

export function validateAutomaticDraft(draft: AutomaticWateringDraft): AutomaticDraftValidationResult {
  const errors: Record<string, string> = {}

  const evaluation = minutesInputToSeconds(draft.evaluation_interval_minutes)
  const cooldown = hoursInputToSeconds(draft.cooldown_hours)
  const maxAge = minutesInputToSeconds(draft.max_metric_age_minutes)
  const timeout = parseFiniteNumber(draft.prometheus_timeout_seconds)
  const expiry = minutesInputToSeconds(draft.message_expiry_minutes)

  if (evaluation === undefined) errors.evaluation_interval_minutes = 'Evaluation interval is required.'
  if (cooldown === undefined) errors.cooldown_hours = 'Cooldown is required.'
  if (maxAge === undefined) errors.max_metric_age_minutes = 'Maximum metric age is required.'
  if (timeout === undefined) errors.prometheus_timeout_seconds = 'Prometheus timeout is required.'
  if (expiry === undefined) errors.message_expiry_minutes = 'Message expiry is required.'

  if (evaluation !== undefined) {
    enforceBounds(
      'Evaluation interval (seconds)',
      evaluation,
      BOUNDS.evaluationIntervalSeconds.min,
      BOUNDS.evaluationIntervalSeconds.max,
      errors,
      'evaluation_interval_minutes',
    )
  }

  if (cooldown !== undefined) {
    enforceBounds('Cooldown (seconds)', cooldown, BOUNDS.cooldownSeconds.min, BOUNDS.cooldownSeconds.max, errors, 'cooldown_hours')
  }

  if (maxAge !== undefined) {
    enforceBounds(
      'Maximum metric age (seconds)',
      maxAge,
      BOUNDS.maxMetricAgeSeconds.min,
      BOUNDS.maxMetricAgeSeconds.max,
      errors,
      'max_metric_age_minutes',
    )
  }

  if (timeout !== undefined) {
    if (!Number.isInteger(timeout)) {
      errors.prometheus_timeout_seconds = 'Prometheus timeout must be a whole number of seconds.'
    } else {
      enforceBounds(
        'Prometheus timeout (seconds)',
        timeout,
        BOUNDS.prometheusTimeoutSeconds.min,
        BOUNDS.prometheusTimeoutSeconds.max,
        errors,
        'prometheus_timeout_seconds',
      )
    }
  }

  if (expiry !== undefined) {
    enforceBounds(
      'Message expiry (seconds)',
      expiry,
      BOUNDS.messageExpirySeconds.min,
      BOUNDS.messageExpirySeconds.max,
      errors,
      'message_expiry_minutes',
    )
  }

  if (evaluation !== undefined && maxAge !== undefined && maxAge < evaluation) {
    errors.max_metric_age_minutes = 'Maximum metric age must be greater than or equal to the evaluation interval.'
  }

  const moistureMetric = draft.moisture_metric.trim()
  if (!moistureMetric) {
    errors.moisture_metric = 'Metric name is required.'
  } else if (!METRIC_IDENTIFIER.test(moistureMetric)) {
    errors.moisture_metric = 'Metric name must be a valid Prometheus metric identifier.'
  }

  const plantLabel = draft.plant_label.trim()
  if (!plantLabel) {
    errors.plant_label = 'Plant label key is required.'
  } else if (!LABEL_KEY_IDENTIFIER.test(plantLabel)) {
    errors.plant_label = 'Plant label key must be a valid Prometheus label identifier.'
  }

  const parsedPlants = {} as Record<AutomaticPlantKey, AutomaticWateringPlantConfig>
  const enabledMetricValueUsage = new Map<string, AutomaticPlantKey[]>()
  for (const plant of AUTOMATIC_PLANTS) {
    const source = draft.plants[plant]
    const threshold = parseFiniteNumber(source.threshold_percent)
    if (threshold === undefined) {
      errors[`plants.${plant}.threshold_percent`] = 'Threshold is required.'
    } else {
      enforceBounds(
        `${plant} threshold`,
        threshold,
        BOUNDS.thresholdPercent.min,
        BOUNDS.thresholdPercent.max,
        errors,
        `plants.${plant}.threshold_percent`,
      )
    }

    const labelValue = source.metric_label_value.trim()
    if (!labelValue) {
      errors[`plants.${plant}.metric_label_value`] = 'Metric label value is required.'
    } else if (!SAFE_LABEL_VALUE.test(labelValue)) {
      errors[`plants.${plant}.metric_label_value`] = 'Metric label value must be safe and non-empty (max 64 chars).'
    }

    parsedPlants[plant] = {
      enabled: source.enabled,
      threshold_percent: threshold ?? 0,
      metric_label_value: labelValue,
    }

    if (source.enabled && labelValue) {
      const normalized = labelValue.toLowerCase()
      enabledMetricValueUsage.set(normalized, [...(enabledMetricValueUsage.get(normalized) ?? []), plant])
    }
  }

  for (const [value, plants] of enabledMetricValueUsage.entries()) {
    if (plants.length <= 1) continue
    const list = plants.join(', ')
    for (const plant of plants) {
      errors[`plants.${plant}.metric_label_value`] = `Enabled plants must have unique metric label values. Duplicate "${value}" is used by: ${list}.`
    }
    errors.plants_metric_label_value_unique = `Enabled plants must have unique metric label values. Duplicate "${value}" is used by: ${list}.`
  }

  if (Object.keys(errors).length > 0) {
    return { errors }
  }

  return {
    errors: {},
    parsedConfig: {
      enabled: draft.enabled,
      evaluation_interval_seconds: evaluation!,
      cooldown_seconds: cooldown!,
      max_metric_age_seconds: maxAge!,
      prometheus_timeout_seconds: timeout!,
      message_expiry_seconds: expiry!,
      moisture_metric: moistureMetric,
      plant_label: plantLabel,
      plants: parsedPlants,
    },
  }
}

function stableConfig(config: AutomaticWateringConfig): string {
  return JSON.stringify({
    ...config,
    moisture_metric: config.moisture_metric.trim(),
    plant_label: config.plant_label.trim(),
    plants: AUTOMATIC_PLANTS.reduce((acc, plant) => {
      const item = config.plants[plant]
      acc[plant] = {
        enabled: item.enabled,
        threshold_percent: item.threshold_percent,
        metric_label_value: item.metric_label_value.trim(),
      }
      return acc
    }, {} as Record<AutomaticPlantKey, AutomaticWateringPlantConfig>),
  })
}

export function isAutomaticDraftDirty(baseConfig: AutomaticWateringConfig, draft: AutomaticWateringDraft): boolean {
  const validated = validateAutomaticDraft(draft)
  if (!validated.parsedConfig) return true
  return stableConfig(baseConfig) !== stableConfig(validated.parsedConfig)
}

export function requiresEnableConfirmation(previousEnabled: boolean, nextEnabled: boolean): boolean {
  return !previousEnabled && nextEnabled
}

export function formatDateTime(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return '—'
  return parsed.toLocaleString()
}

export function formatDurationSeconds(value?: number): string {
  if (value === undefined || !Number.isFinite(value) || value < 0) return '—'
  const rounded = Math.round(value)
  const hours = Math.floor(rounded / 3600)
  const minutes = Math.floor((rounded % 3600) / 60)
  const seconds = rounded % 60
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

export function sampleAgeSeconds(lastEvaluatedAt?: string, lastSampleAt?: string): number | undefined {
  if (!lastEvaluatedAt || !lastSampleAt) return undefined
  const evaluated = new Date(lastEvaluatedAt)
  const sample = new Date(lastSampleAt)
  if (Number.isNaN(evaluated.getTime()) || Number.isNaN(sample.getTime())) return undefined
  const deltaSeconds = Math.round((evaluated.getTime() - sample.getTime()) / 1000)
  return deltaSeconds >= 0 ? deltaSeconds : undefined
}

export function formatDecision(decision?: string): string {
  if (!decision) return 'No decision recorded'
  return decision
}
