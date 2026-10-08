import { useCallback, useEffect, useState } from 'react'
import Chat from '../components/Chat'
import FriendsPanel from '../components/FriendsPanel'
import { api, errorMessage, setToken } from '../lib/api'
import { navigate } from '../lib/router'
import type { FriendsResponse, User } from '../lib/types'
import { useSocket } from '../lib/useSocket'

export default function Me() {
  const [me, setMe] = useState<User | null>(null)
  const [friends, setFriends] = useState<FriendsResponse | null>(null)
  const [selected, setSelected] = useState<User | null>(null)
  const [error, setError] = useState<string | null>(null)
  const socket = useSocket()

  const loadFriends = useCallback(() => {
    api<FriendsResponse>('/friends')
      .then(setFriends)
      .catch((err) => setError(errorMessage(err)))
  }, [])

  useEffect(() => {
    api<User>('/users/@me')
      .then(setMe)
      .catch((err) => setError(errorMessage(err)))
    loadFriends()
  }, [loadFriends])

  function logout() {
    setToken(null)
    navigate('/login')
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <p>
          Logged in as <b>{me?.username ?? '…'}</b> <button onClick={logout}>Log out</button>
        </p>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        {friends && (
          <FriendsPanel data={friends} selected={selected} onSelect={setSelected} onChanged={loadFriends} />
        )}
      </aside>
      <main className="chat">
        {me && selected ? (
          <Chat key={selected.id} me={me} peer={selected} socket={socket} />
        ) : (
          <p>Select a friend to start chatting.</p>
        )}
      </main>
    </div>
  )
}
