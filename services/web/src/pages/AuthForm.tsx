import { useState, type FormEvent } from 'react'
import { api, errorMessage, setToken } from '../lib/api'
import { navigate } from '../lib/router'
import type { AuthResponse } from '../lib/types'

interface Props {
  mode: 'login' | 'register'
}

/** One component serves both /login and /register: the only difference is the endpoint. */
export default function AuthForm({ mode }: Props) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const isLogin = mode === 'login'

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const res = await api<AuthResponse>(`/auth/${mode}`, {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      })
      setToken(res.token)
      navigate('/@me')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="box" onSubmit={onSubmit}>
      <h2>{isLogin ? 'Log in' : 'Register'}</h2>
      <label>
        Username{' '}
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          required
        />
      </label>
      <label>
        Password{' '}
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete={isLogin ? 'current-password' : 'new-password'}
          required
        />
      </label>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <button type="submit" disabled={busy}>
        {isLogin ? 'Log in' : 'Create account'}
      </button>
      <p>
        {isLogin ? 'No account? ' : 'Already registered? '}
        <button type="button" className="link" onClick={() => navigate(isLogin ? '/register' : '/login')}>
          {isLogin ? 'Register' : 'Log in'}
        </button>
      </p>
    </form>
  )
}
