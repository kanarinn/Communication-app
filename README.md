# Communication app

A small Telegram-like app used to learn microservices: register/login, add
friends, and chat with them in real time.

## Architecture

```
browser ──► nginx (web, :80) ──► gateway ──┬──► auth ──► postgres (db: auth)
             static React build            └──► chat ──► postgres (db: chat)
                                                    └──► redis (pub/sub)
```

| Service   | Language      | Owns                                             |
|-----------|---------------|--------------------------------------------------|
| `web`     | React + TS    | Static SPA served by nginx; proxies `/api`, `/ws` |
| `gateway` | Go            | JWT verification, routing to services            |
| `auth`    | Go + Postgres | Users, passwords (bcrypt), issuing JWTs           |
| `chat`    | Go + Postgres + Redis | Friend requests, messages, WebSocket hub |

Only `web` (port 80) and `gateway` (port 8080, for local `npm run dev`) are
published to the host. `auth`, `chat`, `postgres` and `redis` are reachable
solely on the compose network.

### Request flow

1. The browser calls `/api/...` on nginx, which forwards to the gateway.
2. The gateway strips `/api`, verifies the `Authorization: Bearer <jwt>`
   header (or `?token=` for WebSockets) and sets `X-User-ID` on the
   forwarded request. Services trust that header because only the gateway can
   reach them.
3. `/auth/*` and `/users/*` go to auth; `/friends/*`, `/messages/*` and `/ws`
   go to chat.
4. chat never stores usernames. When it needs them it calls auth's
   `/internal/users...` endpoints, which the gateway does not expose.

### Real-time messaging

Each chat process keeps the WebSockets it accepted in memory. When a user
sends a message the process saves it to Postgres and publishes it to the Redis
channel `chat:messages`. Every chat process subscribes to that channel and
delivers the message to the local connections of the sender and recipient.
This is what lets more than one chat replica run behind the gateway.

## API (via gateway)

| Method | Path                                | Auth | Description                       |
|--------|-------------------------------------|------|-----------------------------------|
| POST   | `/api/auth/register`                | no   | `{username,password}` → `{token,user}` |
| POST   | `/api/auth/login`                   | no   | same                              |
| GET    | `/api/users/@me`                    | yes  | current user                      |
| GET    | `/api/friends`                      | yes  | `{friends, incoming, outgoing}`   |
| POST   | `/api/friends/requests`             | yes  | `{username}` send a request       |
| POST   | `/api/friends/requests/{id}/accept` | yes  |                                   |
| POST   | `/api/friends/requests/{id}/decline`| yes  |                                   |
| GET    | `/api/messages/{userId}?before&limit` | yes | history with a friend           |
| WS     | `/ws?token=<jwt>`                   | yes  | send `{"type":"send","to","body"}`, receive `{"type":"message"|"error",...}` |

## Running

```sh
docker compose up --build
```

Open http://localhost, register two users in two browser windows, send a friend
request from one to the other by username, accept it, click the friend and chat.

Frontend development with hot reload (the compose stack must be running so the
gateway is on :8080):

```sh
cd services/web && npm install && npm run dev
```

Set `JWT_SECRET` in the environment for anything other than local testing.

## Layout

```
infra/db/init.sql          creates the auth and chat databases
infra/nginx/nginx.conf     SPA + reverse proxy config, mounted into web
services/gateway           cmd/gateway, internal/token
services/auth              cmd/auth, internal/{httpapi,users,token}
services/chat              cmd/chat, internal/{httpapi,store,realtime,authclient}
services/web               Vite + React; src/pages, src/components, src/lib
```
