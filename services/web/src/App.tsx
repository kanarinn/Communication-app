import { useEffect } from 'react'
import { useToken } from './lib/api'
import { navigate, usePath } from './lib/router'
import AuthForm from './pages/AuthForm'
import Me from './pages/Me'

function Redirect({ to }: { to: string }) {
  useEffect(() => navigate(to), [to])
  return null
}

export default function App() {
  const path = usePath()
  const authed = useToken() !== null

  switch (path) {
    case '/login':
      return authed ? <Redirect to="/@me" /> : <AuthForm mode="login" />
    case '/register':
      return authed ? <Redirect to="/@me" /> : <AuthForm mode="register" />
    case '/@me':
      return authed ? <Me /> : <Redirect to="/login" />
    default:
      return <Redirect to={authed ? '/@me' : '/login'} />
  }
}
