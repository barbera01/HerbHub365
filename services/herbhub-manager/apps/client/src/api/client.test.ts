import { describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { apiFetch, apiFetchBlob, ApiError } from './client'
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

describe('api fetch', () => {
  it('supports custom headers with auth preservation', async () => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.authDisabled = false
    auth.acquireToken = vi.fn().mockResolvedValue('token-123')

    const mock = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ ok: true }) })
    vi.stubGlobal('fetch', mock)

    await apiFetch('/api/messaging/automatic-watering', {
      method: 'PUT',
      headers: { 'If-Match': '"rev"' },
      body: { config: {}, confirm_enable: false },
    })

    const secondArg = mock.mock.calls[0]?.[1] as RequestInit
    const headers = secondArg.headers as Record<string, string>
    expect(headers.Authorization).toBe('Bearer token-123')
    expect(headers['If-Match']).toBe('"rev"')
    expect(headers['Content-Type']).toBe('application/json')
  })

  it('throws ApiError with status code', async () => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.authDisabled = true

    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 412, json: async () => ({ error: 'stale revision' }) }),
    )

    await expect(apiFetch('/api/messaging/automatic-watering')).rejects.toEqual(expect.any(ApiError))
    await expect(apiFetch('/api/messaging/automatic-watering')).rejects.toMatchObject({
      message: 'stale revision',
      status: 412,
    })
  })
})
