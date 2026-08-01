import {
  type AccountInfo,
  EventType,
  InteractionRequiredAuthError,
  type RedirectRequest,
  type AuthenticationResult,
} from '@azure/msal-browser'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { isSpaAuthDisabled } from '@/lib/runtime-config'
import { getApiScope, isMsalConfigured, msal } from './msal'

const redirectRequest: RedirectRequest = {
  scopes: [getApiScope()],
}

export const useAuthStore = defineStore('auth', () => {
  const account = ref<AccountInfo | null>(null)
  const token = ref<string>('')
  const ready = ref(false)
  const error = ref<string>('')
  const authDisabled = ref(isSpaAuthDisabled())

  const isAuthenticated = computed(() => authDisabled.value || Boolean(account.value && token.value))

  async function initialise() {
    if (authDisabled.value) {
      ready.value = true
      return
    }

    if (!isMsalConfigured()) {
      error.value = 'MSAL runtime config is missing. Check env.js values.'
      ready.value = true
      return
    }

    await msal.initialize()
    const result = await msal.handleRedirectPromise()
    if (result?.account) {
      account.value = result.account
    }

    const existing = msal.getAllAccounts()[0] ?? null
    if (existing) {
      account.value = existing
      await acquireToken()
    }

    msal.addEventCallback((message) => {
      if (message.eventType === EventType.LOGIN_SUCCESS) {
        const payload = message.payload as AuthenticationResult
        account.value = payload.account
      }
    })

    ready.value = true
  }

  async function login() {
    if (authDisabled.value) return
    error.value = ''
    await msal.loginRedirect(redirectRequest)
  }

  async function logout() {
    if (authDisabled.value) return
    if (!account.value) return
    await msal.logoutRedirect({ account: account.value })
  }

  async function acquireToken(): Promise<string> {
    if (authDisabled.value) return ''
    if (!account.value) throw new Error('No account')
    try {
      const result = await msal.acquireTokenSilent({
        account: account.value,
        scopes: [getApiScope()],
      })
      token.value = result.accessToken
      return result.accessToken
    } catch (err) {
      if (err instanceof InteractionRequiredAuthError) {
        await msal.acquireTokenRedirect({
          account: account.value,
          scopes: [getApiScope()],
        })
      }
      throw err
    }
  }

  return {
    account,
    token,
    ready,
    error,
    authDisabled,
    isAuthenticated,
    initialise,
    login,
    logout,
    acquireToken,
  }
})
