import { useState, type FormEvent } from 'react'
import { api, errorMessage } from '../lib/api'
import type { FriendsResponse, User } from '../lib/types'

interface Props {
  data: FriendsResponse
  selected: User | null
  onSelect: (u: User) => void
  onChanged: () => void
}

export default function FriendsPanel({ data, selected, onSelect, onChanged }: Props) {
  const [username, setUsername] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function run(action: () => Promise<unknown>) {
    setError(null)
    try {
      await action()
      onChanged()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  function addFriend(e: FormEvent) {
    e.preventDefault()
    const name = username.trim()
    if (!name) return
    run(() =>
      api('/friends/requests', { method: 'POST', body: JSON.stringify({ username: name }) }),
    ).then(() => setUsername(''))
  }

  const respond = (id: number, verb: 'accept' | 'decline') =>
    run(() => api(`/friends/requests/${id}/${verb}`, { method: 'POST' }))

  return (
    <>
      <form onSubmit={addFriend}>
        <label>
          Add friend{' '}
          <input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="username" />
        </label>{' '}
        <button type="submit">Send request</button>
      </form>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}

      {data.incoming.length > 0 && (
        <>
          <h3>Requests</h3>
          <ul>
            {data.incoming.map((r) => (
              <li key={r.id}>
                {r.user.username}{' '}
                <button onClick={() => respond(r.id, 'accept')}>Accept</button>{' '}
                <button onClick={() => respond(r.id, 'decline')}>Decline</button>
              </li>
            ))}
          </ul>
        </>
      )}

      <h3>
        Friends <button onClick={onChanged}>Refresh</button>
      </h3>
      {data.friends.length === 0 && <p>No friends yet.</p>}
      <ul>
        {data.friends.map((f) => (
          <li key={f.id}>
            {selected?.id === f.id ? (
              <b>{f.username}</b>
            ) : (
              <button className="link" onClick={() => onSelect(f)}>
                {f.username}
              </button>
            )}
          </li>
        ))}
      </ul>

      {data.outgoing.length > 0 && (
        <>
          <h3>Sent requests</h3>
          <ul>
            {data.outgoing.map((r) => (
              <li key={r.id}>{r.user.username} (pending)</li>
            ))}
          </ul>
        </>
      )}
    </>
  )
}
