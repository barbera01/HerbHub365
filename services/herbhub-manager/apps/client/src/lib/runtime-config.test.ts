import { describe, expect, it } from 'vitest'
import { getRuntimeConfig, isSpaAuthDisabled } from './runtime-config'

describe('runtime config', () => {
  it('defaults auth disabled to false', () => {
    const cfg = getRuntimeConfig()
    expect(cfg.auth.disabled).toBe(false)
    expect(isSpaAuthDisabled()).toBe(false)
  })

  it('reads runtime injected auth disabled', () => {
    const g = globalThis as any
    g.window = {
      location: { origin: 'http://localhost:5173' } as Location,
      __HERBHUB_MANAGER_CONFIG__: {
        auth: {
          disabled: true,
        },
      },
    }
    expect(getRuntimeConfig().auth.disabled).toBe(true)
    expect(isSpaAuthDisabled()).toBe(true)
  })
})
