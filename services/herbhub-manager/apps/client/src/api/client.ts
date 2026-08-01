import { useAuthStore } from '@/session/auth'

type FetchOptions = {
  method?: 'GET' | 'POST' | 'DELETE'
  body?: unknown
}

export async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
  const auth = useAuthStore()
  const token = auth.authDisabled ? '' : await auth.acquireToken()

  const headers: Record<string, string> = {
    ...(options.body ? { 'Content-Type': 'application/json' } : {}),
  }
  if (token) headers.Authorization = `Bearer ${token}`

  const res = await fetch(path, {
    method: options.method ?? 'GET',
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
  })

  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const data = (await res.json()) as { error?: string }
      if (data.error) message = data.error
    } catch {
      // ignore
    }
    throw new Error(message)
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export async function apiFetchBlob(path: string): Promise<Blob> {
  const auth = useAuthStore()
  const token = auth.authDisabled ? '' : await auth.acquireToken()
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`

  const res = await fetch(path, {
    method: 'GET',
    headers,
  })

  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const data = (await res.json()) as { error?: string }
      if (data.error) message = data.error
    } catch {
      // ignore
    }
    throw new Error(message)
  }

  return await res.blob()
}
