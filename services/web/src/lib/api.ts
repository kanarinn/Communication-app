import { useSyncExternalStore } from 'react'
import { navigate } from './router'

const TOKEN_KEY = 'token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

// Components must not call getToken() directly during render: the React
// Compiler would cache the result forever. Changes are announced through
// these listeners so useToken() re-renders whoever depends on the token.
const tokenListeners = new Set<() => void>()

export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
  tokenListeners.forEach((l) => l())
}

function subscribeToken(cb: () => void) {
  tokenListeners.add(cb)
  return () => {
    tokenListeners.delete(cb)
  }
}

/** Reactive token for components: re-renders on login and logout. */
export function useToken(): string | null {
  return useSyncExternalStore(subscribeToken, getToken)
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

/** Calls the gateway. All backend paths live under /api. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`/api${path}`, { ...init, headers })
  if (res.status === 204) return undefined as T

  const data = (await res.json().catch(() => ({}))) as { error?: string }
  if (!res.ok) {
    if (res.status === 401) {
      // Token missing, expired or forged: back to login.
      setToken(null)
      navigate('/login')
    }
    throw new ApiError(res.status, data.error ?? res.statusText)
  }
  return data as T
}

export function wsUrl(): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}/ws?token=${encodeURIComponent(getToken() ?? '')}`
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Something went wrong'
}
