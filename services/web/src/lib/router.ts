// A tiny history-based router: enough for /login, /register and /@me
// without adding a dependency.
import { useSyncExternalStore } from 'react'

const listeners = new Set<() => void>()

function subscribe(cb: () => void) {
  listeners.add(cb)
  window.addEventListener('popstate', cb)
  return () => {
    listeners.delete(cb)
    window.removeEventListener('popstate', cb)
  }
}

export function usePath(): string {
  return useSyncExternalStore(subscribe, () => window.location.pathname)
}

export function navigate(path: string) {
  if (path === window.location.pathname) return
  window.history.pushState(null, '', path)
  listeners.forEach((l) => l())
}
