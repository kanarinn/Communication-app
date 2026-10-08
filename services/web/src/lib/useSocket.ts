import { useEffect, useState } from 'react'
import { wsUrl } from './api'

export function useSocket(): WebSocket | null {
  const [socket, setSocket] = useState<WebSocket | null>(null)

  useEffect(() => {
    let ws: WebSocket
    let timer: number | undefined
    let unmounted = false

    function connect() {
      ws = new WebSocket(wsUrl())
      ws.onopen = () => setSocket(ws)
      ws.onclose = () => {
        setSocket(null)
        if (!unmounted) timer = window.setTimeout(connect, 2000)
      }
    }
    connect()

    return () => {
      unmounted = true
      window.clearTimeout(timer)
      ws.close()
    }
  }, [])

  return socket
}
