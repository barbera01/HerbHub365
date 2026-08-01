import { describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { apiFetchBlob } from './client'
import { useAuthStore } from '@/session/auth'

describe('api blob fetch', () => {
  it('omits auth header when auth disabled', async () => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.authDisabled = true

    const mock = vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(['ok']) })
    vi.stubGlobal('fetch', mock)

    await apiFetchBlob('/api/videos/file.mp4')
    expect(mock).toHaveBeenCalled()
    const secondArg = mock.mock.calls[0]?.[1] as RequestInit
    expect((secondArg.headers as Record<string, string>).Authorization).toBeUndefined()
  })
})
