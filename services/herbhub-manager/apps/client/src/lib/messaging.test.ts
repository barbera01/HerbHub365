import { describe, expect, it } from 'vitest'
import {
  canProvisionCatalogue,
  derivePublishRoute,
  getPublishReadinessBlockReason,
  normalizeMessagingOverview,
  normalizeMessagingPublishResult,
  parsePayloadObject,
  resetPublishConfirmationState,
  requiresDangerConfirmation,
  validateWateringPayload,
} from './messaging'

describe('messaging payload parsing', () => {
  it('rejects malformed json', () => {
    const parsed = parsePayloadObject('{')
    expect(parsed.error).toContain('valid JSON')
  })

  it('rejects non-object json', () => {
    const parsed = parsePayloadObject('[]')
    expect(parsed.error).toContain('JSON object')
  })

  it('accepts object json', () => {
    const parsed = parsePayloadObject('{"plant":"basil"}')
    expect(parsed.value).toEqual({ plant: 'basil' })
  })
})

describe('watering validation', () => {
  it('validates plant/action/value for watering-water', () => {
    const errors = validateWateringPayload('watering-water', {
      plant: 'mint',
      action: 'skip',
      value: 120,
    })
    expect(errors).toHaveLength(3)
  })

  it('accepts valid watering payload', () => {
    const errors = validateWateringPayload('watering-water', {
      plant: 'basil',
      action: 'water',
      value: 35.2,
    })
    expect(errors).toEqual([])
  })
})

describe('route derivation and confirmation helpers', () => {
  it('derives watering route from plant', () => {
    expect(derivePublishRoute('watering-water', { plant: 'Chilli' })).toBe('watering.chilli')
  })

  it('uses default/fallback route for plant health', () => {
    expect(derivePublishRoute('plant-health-json', {}, '')).toBe('plant.health.left')
    expect(derivePublishRoute('plant-health-json', {}, 'plant.health.right')).toBe('plant.health.right')
  })

  it('captures confirmation behavior', () => {
    expect(
      requiresDangerConfirmation({
        id: 'watering-water',
        name: 'Water plant',
        catalogue_id: 'watering',
        description: '',
        requires_confirmation: true,
        default_payload: {},
      }),
    ).toBe(true)
    expect(
      requiresDangerConfirmation({
        id: 'watering-skip',
        name: 'Skip watering',
        catalogue_id: 'watering',
        description: '',
        requires_confirmation: false,
        default_payload: {},
      }),
    ).toBe(false)
  })

  it('allows provisioning only for missing state', () => {
    expect(canProvisionCatalogue('missing')).toBe(true)
    expect(canProvisionCatalogue('ready')).toBe(false)
    expect(canProvisionCatalogue('drifted')).toBe(false)
    expect(canProvisionCatalogue('unavailable')).toBe(false)
  })
})

describe('messaging response normalization', () => {
  it('normalizes overview from backend snake_case shape', () => {
    const normalized = normalizeMessagingOverview({
      enabled: true,
      broker_status: 'ok',
      grafana_url: 'https://grafana.example',
      catalogues: [{ id: 'watering', name: 'Watering', state: 'ready', queues: { 'watering.queue': { ready: 2, unacked: 1, consumers: 1 } } }],
      templates: [
        {
          id: 'plant-health-json',
          name: 'Plant health JSON',
          catalogue_id: 'plant-health',
          description: 'desc',
          requires_confirmation: false,
          allowed_routing_keys: ['plant.health.left'],
          default_routing_key: 'plant.health.left',
          default_payload: { source: 'manager' },
        },
      ],
      prometheus: { available: true, queues: { 'watering.queue': { ready: 2, unacked: 1, consumers: 1 } } },
    })

    expect(normalized.enabled).toBe(true)
    expect(normalized.broker_status).toBe('ok')
    expect(normalized.catalogues[0]?.queues?.['watering.queue']?.ready).toBe(2)
    expect(normalized.templates[0]?.catalogue_id).toBe('plant-health')
    expect(normalized.prometheus?.available).toBe(true)
  })

  it('normalizes publish result', () => {
    const normalized = normalizeMessagingPublishResult({
      message_id: 'abc',
      exchange: 'herbhub.watering',
      routing_key: 'watering.basil',
      routed: true,
    })
    expect(normalized).toEqual({
      message_id: 'abc',
      exchange: 'herbhub.watering',
      routing_key: 'watering.basil',
      routed: true,
    })
  })
})

describe('publish readiness gating', () => {
  const template = {
    id: 'watering-water',
    name: 'Water plant',
    catalogue_id: 'watering',
    description: '',
    requires_confirmation: true,
    default_payload: {},
  }

  it('blocks when messaging disabled', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: false,
        broker_status: 'ok',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'ready' }],
        templates: [],
      },
      template,
    })
    expect(reason).toContain('messaging is disabled')
  })

  it('blocks when broker status is not ok', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'unavailable',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'ready' }],
        templates: [],
      },
      template,
    })
    expect(reason).toContain('broker status is unavailable')
  })

  it('blocks when selected template catalogue is not ready', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'ok',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'missing' }],
        templates: [],
      },
      template,
    })
    expect(reason).toContain('is missing')
  })

  it('allows publish when enabled, broker ok, and catalogue ready', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'ok',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'ready', queues: { 'watering.queue': { ready: 0, unacked: 0, consumers: 1 } } }],
        templates: [],
      },
      template,
    })
    expect(reason).toBe('')
  })

  it('blocks watering-water publish when watering consumer count is zero', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'ok',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'ready', queues: { 'watering.queue': { ready: 0, unacked: 0, consumers: 0 } } }],
        templates: [],
      },
      template,
    })
    expect(reason).toContain('no active watering consumer')
  })

  it('blocks watering-water publish when more than one consumer is active', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'ok',
        catalogues: [{ id: 'watering', name: 'Watering', state: 'ready', queues: { 'watering.queue': { ready: 0, unacked: 0, consumers: 2 } } }],
        templates: [],
      },
      template,
    })
    expect(reason).toContain('expected exactly one active watering consumer, but found 2')
  })

  it('does not block non-watering-water templates when consumer count is zero', () => {
    const reason = getPublishReadinessBlockReason({
      overview: {
        enabled: true,
        broker_status: 'ok',
        catalogues: [{ id: 'plant-health', name: 'Plant Health', state: 'ready', queues: { 'plant.health.analysis': { ready: 0, unacked: 0, consumers: 0 } } }],
        templates: [],
      },
      template: {
        id: 'plant-health-json',
        name: 'Plant health JSON',
        catalogue_id: 'plant-health',
        description: '',
        requires_confirmation: false,
        default_payload: {},
      },
    })
    expect(reason).toBe('')
  })
})

describe('confirmation reset helper', () => {
  it('resets both confirmation checkboxes to false', () => {
    expect(resetPublishConfirmationState()).toEqual({ publishConfirmed: false, dangerConfirmed: false })
  })
})
