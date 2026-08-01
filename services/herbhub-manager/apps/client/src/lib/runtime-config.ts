export type RuntimeConfig = {
  auth: {
    disabled: boolean
    authority: string
    clientId: string
    redirectUri: string
    postLogoutRedirectUri: string
    apiScope: string
    apiAudience: string
    requiredRole: string
  }
}

const fallback: RuntimeConfig = {
  auth: {
    disabled: false,
    authority: '',
    clientId: '',
    redirectUri: typeof window !== 'undefined' ? window.location.origin : 'http://localhost:5173',
    postLogoutRedirectUri: typeof window !== 'undefined' ? window.location.origin : 'http://localhost:5173',
    apiScope: '',
    apiAudience: '',
    requiredRole: 'Manager.Operator',
  },
}

export function getRuntimeConfig(): RuntimeConfig {
  const loaded = typeof window !== 'undefined' ? window.__HERBHUB_MANAGER_CONFIG__ : undefined
  return {
    auth: {
      ...fallback.auth,
      ...(loaded?.auth ?? {}),
    },
  }
}

export function isSpaAuthDisabled(): boolean {
  return getRuntimeConfig().auth.disabled === true
}
