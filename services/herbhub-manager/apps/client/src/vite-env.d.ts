/// <reference types="vite/client" />

interface Window {
  __HERBHUB_MANAGER_CONFIG__?: {
    auth?: {
      disabled?: boolean
      authority?: string
      clientId?: string
      redirectUri?: string
      postLogoutRedirectUri?: string
      apiScope?: string
      apiAudience?: string
      requiredRole?: string
    }
  }
}
