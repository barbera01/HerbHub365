import { describe, expect, it } from 'vitest'
import {
  createAutomaticDraft,
  formatDateTime,
  formatDecision,
  formatDurationSeconds,
  hoursInputToSeconds,
  isAutomaticDraftDirty,
  minutesInputToSeconds,
  normalizeAutomaticWateringRepresentation,
  requiresEnableConfirmation,
  sampleAgeSeconds,
  sanitizeTextForUi,
  secondsToHoursInput,
  secondsToMinutesInput,
  shouldResetEnableConfirmation,
  validateAutomaticDraft,
} from './automatic-watering'
import type { AutomaticWateringConfig } from '@/types/api'

const baseConfig: AutomaticWateringConfig = {
  enabled: false,
  evaluation_interval_seconds: 300,
  cooldown_seconds: 21600,
  max_metric_age_seconds: 900,
  prometheus_timeout_seconds: 10,
  message_expiry_seconds: 300,
  moisture_metric: 'herbhub_soil_percent',
  plant_label: 'plant',
  plants: {
    basil: { enabled: true, threshold_percent: 30, metric_label_value: 'basil' },
    chilli: { enabled: true, threshold_percent: 30, metric_label_value: 'chilli' },
    oregano: { enabled: true, threshold_percent: 30, metric_label_value: 'oregano' },
  },
}

describe('automatic watering normalization', () => {
  it('normalizes backend representation shape', () => {
    const normalized = normalizeAutomaticWateringRepresentation({
      config_revision: 12,
      config: baseConfig,
      runtime_status: {
        running: true,
        faulted: false,
        next_evaluation_at: '2026-08-11T10:00:00Z',
        instance_id: 'manager-a',
      },
      state: {
        basil: {
          last_evaluated_at: '2026-08-11T09:55:00Z',
          last_sample_at: '2026-08-11T09:54:30Z',
          last_value: 29.4,
          last_decision: 'water_published',
          last_message_id: 'msg-1',
        },
      },
    })

    expect(normalized.config_revision).toBe(12)
    expect(normalized.status.running).toBe(true)
    expect(normalized.status.instance_id).toBe('manager-a')
    expect(normalized.status.plants.basil.last_value).toBe(29.4)
    expect(normalized.status.plants.basil.last_message_id).toBe('msg-1')
  })

  it('sanitizes runtime fault/error text', () => {
    const normalized = normalizeAutomaticWateringRepresentation({
      config_revision: 2,
      config: baseConfig,
      runtime_status: { running: false, faulted: true, fault: 'bad\nline\tvalue' },
      state: {
        basil: { last_error: 'oops\nagain' },
      },
    })

    expect(normalized.status.fault).toBe('bad line value')
    expect(normalized.status.plants.basil.last_error).toBe('oops again')
  })
})

describe('automatic watering conversions', () => {
  it('converts minutes and hours to integer seconds safely', () => {
    expect(minutesInputToSeconds('1.5')).toBe(90)
    expect(hoursInputToSeconds('1.5')).toBe(5400)
    expect(minutesInputToSeconds('')).toBeUndefined()
    expect(hoursInputToSeconds('x')).toBeUndefined()
  })

  it('converts seconds to user-friendly inputs', () => {
    expect(secondsToMinutesInput(300)).toBe('5')
    expect(secondsToHoursInput(21600)).toBe('6')
  })
})

describe('automatic watering validation', () => {
  it('validates boundaries and cross-field constraints', () => {
    const draft = createAutomaticDraft(baseConfig)
    draft.evaluation_interval_minutes = '2'
    draft.max_metric_age_minutes = '1'
    draft.cooldown_hours = '0.5'
    draft.prometheus_timeout_seconds = '31'
    draft.message_expiry_minutes = '0.2'
    draft.moisture_metric = 'bad metric'
    draft.plant_label = 'bad-label'
    draft.plants.basil.threshold_percent = '90'
    draft.plants.basil.metric_label_value = 'bad value with spaces'

    const result = validateAutomaticDraft(draft)
    expect(result.parsedConfig).toBeUndefined()
    expect(result.errors.max_metric_age_minutes).toContain('greater than or equal')
    expect(result.errors.cooldown_hours).toContain('between 3600 and 604800')
    expect(result.errors.prometheus_timeout_seconds).toContain('between 1 and 30')
    expect(result.errors.message_expiry_minutes).toContain('between 30 and 300')
    expect(result.errors.moisture_metric).toContain('Prometheus metric identifier')
    expect(result.errors.plant_label).toContain('Prometheus label identifier')
    expect(result.errors['plants.basil.threshold_percent']).toContain('between 5 and 80')
    expect(result.errors['plants.basil.metric_label_value']).toContain('safe and non-empty')
  })

  it('accepts valid draft', () => {
    const result = validateAutomaticDraft(createAutomaticDraft(baseConfig))
    expect(result.errors).toEqual({})
    expect(result.parsedConfig).toBeDefined()
  })

  it('allows backend-safe metric label values including slash', () => {
    const draft = createAutomaticDraft(baseConfig)
    draft.plants.basil.metric_label_value = 'zone/a-1'
    const result = validateAutomaticDraft(draft)
    expect(result.errors['plants.basil.metric_label_value']).toBeUndefined()
    expect(result.parsedConfig).toBeDefined()
  })

  it('requires unique metric label values across enabled plants', () => {
    const draft = createAutomaticDraft(baseConfig)
    draft.plants.basil.metric_label_value = 'left-bed'
    draft.plants.chilli.metric_label_value = 'left-bed'
    draft.plants.oregano.metric_label_value = 'oregano'

    const result = validateAutomaticDraft(draft)
    expect(result.parsedConfig).toBeUndefined()
    expect(result.errors.plants_metric_label_value_unique).toContain('unique metric label values')
    expect(result.errors['plants.basil.metric_label_value']).toContain('Duplicate')
    expect(result.errors['plants.chilli.metric_label_value']).toContain('Duplicate')
  })

  it('does not enforce uniqueness on disabled plants', () => {
    const draft = createAutomaticDraft(baseConfig)
    draft.plants.basil.metric_label_value = 'shared'
    draft.plants.chilli.metric_label_value = 'shared'
    draft.plants.chilli.enabled = false

    const result = validateAutomaticDraft(draft)
    expect(result.errors.plants_metric_label_value_unique).toBeUndefined()
    expect(result.errors['plants.basil.metric_label_value']).toBeUndefined()
    expect(result.parsedConfig).toBeDefined()
  })
})

describe('automatic watering dirty checks and confirmation', () => {
  it('detects dirty vs unchanged drafts', () => {
    const unchangedDraft = createAutomaticDraft(baseConfig)
    expect(isAutomaticDraftDirty(baseConfig, unchangedDraft)).toBe(false)

    const changedDraft = createAutomaticDraft(baseConfig)
    changedDraft.plants.oregano.threshold_percent = '31'
    expect(isAutomaticDraftDirty(baseConfig, changedDraft)).toBe(true)
  })

  it('requires explicit confirmation only when enabling', () => {
    expect(requiresEnableConfirmation(false, true)).toBe(true)
    expect(requiresEnableConfirmation(true, true)).toBe(false)
    expect(requiresEnableConfirmation(true, false)).toBe(false)
  })

  it('resets enable confirmation only on substantive draft edits', () => {
    expect(shouldResetEnableConfirmation({ fieldPath: 'enable_confirmation', isEnableTransitionPending: true })).toBe(false)
    expect(shouldResetEnableConfirmation({ fieldPath: 'draft', isEnableTransitionPending: true })).toBe(true)
    expect(shouldResetEnableConfirmation({ fieldPath: 'draft', isEnableTransitionPending: false })).toBe(false)
  })
})

describe('automatic watering status formatting', () => {
  it('formats durations, decisions, and sample ages', () => {
    expect(formatDurationSeconds(3661)).toBe('1h 1m 1s')
    expect(formatDurationSeconds(61)).toBe('1m 1s')
    expect(formatDurationSeconds(undefined)).toBe('—')
    expect(formatDecision('water_published')).toBe('water_published')
    expect(formatDecision(undefined)).toBe('No decision recorded')
    expect(sampleAgeSeconds('2026-08-11T10:00:00Z', '2026-08-11T09:59:15Z')).toBe(45)
    expect(sampleAgeSeconds(undefined, undefined)).toBeUndefined()
  })

  it('formats datetimes and sanitizes text', () => {
    expect(formatDateTime('2026-08-11T10:00:00Z')).not.toBe('—')
    expect(formatDateTime('not-a-date')).toBe('—')
    expect(sanitizeTextForUi('hello\nworld\t!')).toBe('hello world !')
  })
})
