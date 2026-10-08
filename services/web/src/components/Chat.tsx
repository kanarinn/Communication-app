import { useEffect, useRef, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../lib/api'
import type { Message, ServerEvent, User } from '../lib/types'

interface Props {
  me: User
  peer: User
  socket: WebSocket | null
}

export default function Chat({ me, peer, socket }: Props) {
  const [messages, setMessages] = useState<Message[]>([])
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let cancelled = false
    api<{ messages: Message[] }>(`/messages/${peer.id}`)
      .then((res) => {
        if (!cancelled) setMessages(res.messages)
      })
      .catch((err) => {
        if (!cancelled) setError(errorMessage(err))
      })
    return () => {
      cancelled = true
    }
  }, [peer.id])

  useEffect(() => {
    if (!socket) return
    function onMessage(ev: MessageEvent<string>) {
      const data = JSON.parse(ev.data) as ServerEvent
      if (data.type === 'error') {
        setError(data.error)
        return
      }
      const m = data.message
      // The hub only sends us messages we are part of; keep the ones for this peer.
      if (m.senderId !== peer.id && m.recipientId !== peer.id) return
      setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]))
    }
    socket.addEventListener('message', onMessage)
    return () => socket.removeEventListener('message', onMessage)
  }, [socket, peer.id])

  useEffect(() => {
    bottomRef.current?.scrollIntoView()
  }, [messages])

  function send(e: FormEvent) {
    e.preventDefault()
    const body = draft.trim()
    if (!body || !socket) return
    socket.send(JSON.stringify({ type: 'send', to: peer.id, body }))
    setDraft('')
    setError(null)
  }

  return (
    <>
      <h3>Chat with {peer.username}</h3>
      <div className="messages">
        {messages.length === 0 && <p>No messages yet.</p>}
        {messages.map((m) => (
          <p key={m.id}>
            <b>{m.senderId === me.id ? me.username : peer.username}:</b> {m.body}
          </p>
        ))}
        <div ref={bottomRef} />
      </div>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <form className="compose" onSubmit={send}>
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={socket ? 'Type a message' : 'Connecting…'}
          disabled={!socket}
        />
        <button type="submit" disabled={!socket || !draft.trim()}>
          Send
        </button>
      </form>
    </>
  )
}
