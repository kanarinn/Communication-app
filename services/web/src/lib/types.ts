export interface User {
  id: string
  username: string
}

export interface AuthResponse {
  token: string
  user: User
}

export interface FriendRequest {
  id: number
  user: User
  createdAt: string
}

export interface FriendsResponse {
  friends: User[]
  incoming: FriendRequest[]
  outgoing: FriendRequest[]
}

export interface Message {
  id: number
  senderId: string
  recipientId: string
  body: string
  createdAt: string
}

export type ServerEvent =
  | { type: 'message'; message: Message }
  | { type: 'error'; error: string }
