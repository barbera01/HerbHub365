import { describe, expect, it } from 'vitest'
import { timeAgo } from './utils'

describe('timeAgo', () => {
  it('returns just now for sub-minute timestamps', () => {
    const iso = new Date(Date.now() - 10_000).toISOString()
    expect(timeAgo(iso)).toBe('just now')
  })

  it('returns minute output for recent timestamps', () => {
    const iso = new Date(Date.now() - 2 * 60_000).toISOString()
    expect(timeAgo(iso)).toContain('m ago')
  })
})
