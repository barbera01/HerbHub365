import { describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useManagerStore } from './manager'

vi.mock('@/api/client', () => ({
  apiFetch: async () => {
    throw new Error('offline')
  },
}))

describe('manager store selection', () => {
  it('toggles selected post slugs', () => {
    setActivePinia(createPinia())
    const store = useManagerStore()

    expect(store.selected.size).toBe(0)
    store.toggleSelected('post-a')
    expect(store.selected.has('post-a')).toBe(true)
    store.toggleSelected('post-a')
    expect(store.selected.has('post-a')).toBe(false)
  })

  it('safe loaders capture errors without throwing', async () => {
    setActivePinia(createPinia())
    const store = useManagerStore()
    await expect(store.loadJobsSafe()).resolves.toBeUndefined()
    expect(store.error).toContain('offline')
  })
})
