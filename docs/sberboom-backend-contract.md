# SberBoom Skill Backend — WebSocket Contract

This document describes the WebSocket API between the **SberBoom skill backend** (server) and **PicoClaw running on the SberBoom device** (client).

The skill backend implementation is out of scope here — this contract defines only the interface both sides must conform to.

---

## Connection

- PicoClaw establishes an outbound WSS connection to the backend URL configured in `sberboom.backend_url`.
- Optional Bearer token authentication: PicoClaw includes `Authorization: Bearer <token>` in the HTTP upgrade headers if `sberboom.token` is set.
- After a successful handshake, PicoClaw **must** send a `register` message within **5 seconds**, otherwise the backend may close the connection.

---

## Wire Format

All messages are JSON objects. Every message has a mandatory `type` field.

```json
{
  "type": "<message-type>",
  "timestamp": 1712345678901,
  ...type-specific fields...
}
```

`timestamp` is Unix milliseconds (optional but recommended).

---

## Messages: PicoClaw → Backend

### `register`

Sent immediately after connect (and again on every reconnect) to bind this WebSocket connection to a Sber user.

```json
{
  "type": "register",
  "user_id": "sber-user-001",
  "token": "optional-secret",
  "timestamp": 1712345678901
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `user_id` | yes | Sber platform user identifier for this device |
| `token` | no | Shared secret for additional backend-side auth |

**Backend behaviour:** Associate this WebSocket connection with `user_id`. On re-registration with the same `user_id`, replace the previous connection reference. Respond with nothing (or optionally with a `pong`).

---

### `end_session`

PicoClaw requests graceful termination of the active Sber skill session.

```json
{
  "type": "end_session",
  "timestamp": 1712345678901
}
```

**Backend behaviour:** On the next webhook call from the Sber platform for this user, return `"end_session": true` in the response.

---

### `ping`

WebSocket keepalive. The backend should respond with a `pong`, or rely on the native WebSocket ping/pong mechanism.

```json
{ "type": "ping" }
```

---

## Messages: Backend → PicoClaw

### `message`

A transcribed user utterance relayed from the Sber platform.

```json
{
  "type": "message",
  "id": "uuid-of-the-sber-request",
  "text": "что такое квантовые компьютеры",
  "session_id": "sber-session-uuid",
  "timestamp": 1712345678901
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | yes | Unique message ID (can be the Sber `messageId`) |
| `text` | yes | ASR-transcribed user speech, plain text |
| `session_id` | no | Sber session identifier for this dialog |

**PicoClaw behaviour:** Forwards `text` to the agent. The agent response is played via the local TTS script; **no response is sent back to the backend over WebSocket**.

---

### `session_end`

The backend signals that the Sber session has ended (user said "Хватит", timeout, or platform-initiated close).

```json
{
  "type": "session_end",
  "timestamp": 1712345678901
}
```

**PicoClaw behaviour:** Logs the event. The WebSocket connection remains open for the next session.

---

### `error`

The backend reports an error condition.

```json
{
  "type": "error",
  "code": "unknown_user",
  "message": "No registered connection for this user ID",
  "timestamp": 1712345678901
}
```

| `code` value | Meaning |
|---|---|
| `auth_failed` | Invalid or missing token |
| `unknown_user` | `register` not received before first `message` |
| `internal` | Unexpected backend error |

---

### `pong`

Response to a `ping` message or WebSocket-level pong frame.

```json
{ "type": "pong" }
```

---

## Reconnection Behaviour

- PicoClaw reconnects automatically after any connection drop with a fixed 5-second backoff.
- On reconnect, PicoClaw re-sends `register` with the same `user_id`.
- **The backend must accept re-registration** and update its connection reference gracefully.
- Messages sent to a user while their WebSocket is disconnected are **dropped** — the backend does not buffer them.

---

## Full Session Flow

```
[User] "Запусти навык PicoClaw"
   ↓
[Sber Platform] activates skill, opens session for user sber-user-001
   ↓
[Skill Backend] receives POST /webhook:
  {
    "messageId": "msg-001",
    "sessionId": "sess-abc",
    "userId": "sber-user-001",
    "payload": { "message": { "text": "привет" } }
  }
   ↓
[Skill Backend] looks up WebSocket for sber-user-001
[Skill Backend] → WebSocket: { "type": "message", "id": "msg-001", "text": "привет", "session_id": "sess-abc" }
[Skill Backend] → HTTP 200 to Sber: { "status": "ok", "actions": [], "end_session": false }
   ↓
[PicoClaw] receives message, agent processes it
[PicoClaw] → ./tts.sh "Привет! Чем могу помочь?"   (local TTS playback)
   ↓
[User hears response, speaks again]
   ↓
[Sber Platform] → POST /webhook (next utterance) ...
```

The dialog continues until:
- The user says "Хватит" (Sber built-in termination), OR
- PicoClaw sends `end_session` → backend returns `"end_session": true` on the next webhook response.

---

## Configuration Reference (PicoClaw side)

```json
{
  "channels": {
    "sberboom": {
      "enabled": true,
      "backend_url": "wss://skill.example.com/ws",
      "user_id": "sber-user-001",
      "tts_script": "./scripts/sberboom/tts.sh",
      "token": "optional-shared-secret",
      "ping_interval": 30,
      "read_timeout": 60,
      "allow_from": ["*"]
    }
  }
}
```
