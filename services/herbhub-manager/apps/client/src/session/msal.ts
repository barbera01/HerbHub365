import { PublicClientApplication } from '@azure/msal-browser'
import { getRuntimeConfig, isSpaAuthDisabled } from '@/lib/runtime-config'

const cfg = getRuntimeConfig()

export const msal = new PublicClientApplication({
  auth: {
    authority: cfg.auth.authority,
    clientId: cfg.auth.clientId,
    redirectUri: cfg.auth.redirectUri,
    postLogoutRedirectUri: cfg.auth.postLogoutRedirectUri,
  },
  cache: {
    cacheLocation: 'localStorage',
  },
})

export function isMsalConfigured(): boolean {
  if (isSpaAuthDisabled()) return false
  return Boolean(cfg.auth.authority && cfg.auth.clientId && cfg.auth.apiScope)
}

export function getApiScope(): string {
  return cfg.auth.apiScope
}
