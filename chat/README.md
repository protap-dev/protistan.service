# Chat Service

## Overview
- Encore service that provides real-time messaging between customers and artisans once a booking is assigned.
- Threads are created automatically from booking domain events; system status updates are injected via the event subscriber.
- Persists data in the `chat` Postgres database (migrations in `./migrations`) and emits chat-domain events through the shared outbox relay.

## Architecture
- `domain/`: core entities (`Thread`, `Message`), validators, automated message templates, and event contracts.
- `handlers/`: HTTP + WebSocket handlers, DTOs, and the in-process WebSocket hub.
- `events/`: booking topic subscribers that create threads and push system messages.
- `repository/`: GORM-backed repositories for threads and messages (includes optimistic locking + idempotency checks).
- `relay/`: wrapper around the shared outbox to publish chat events to other services.
- `internal/`: auth, authz, logging, error normalization, pagination, and relay configuration helpers.
- External dependencies: Encore runtime, GORM (Postgres driver), `gorilla/websocket`.

## Data Model
| Resource | Key fields |
| --- | --- |
| Thread | `id`, `booking_id`, `customer_id`, `artisan_id`, `last_message_at`, optional metadata (stores `artisan_user_id`). |
| Message | `id`, `thread_id`, `sender_id`, `content`, `message_type` (`user`, `system`, `status_update`), `status` (`sending` → `sent` → `delivered` → `read`), `idempotency_key`, timestamps (`sent_at`, `delivered_at`, `read_at`). |

## HTTP APIs (All require standard Protisan auth headers)

### List Threads
- **GET** `/v0/chat/threads`
- Optional query: `limit` (default 20, max 100), `offset` (default 0).
- Returns threads that include the authenticated user.

### List Messages
- **GET** `/v0/chat/threads/{threadId}/messages`
- Optional query: `limit` (default 50, max 100), `offset` (default 0).
- Returns newest messages first (server normalizes pagination).

### Send Message
- **POST** `/v0/chat/threads/{threadId}/messages`
- Body:
```json
{
  "content": "Hi there, thanks for accepting the job!",
  "idempotency_key": "customer-uuid-1699446434"
}
```
- Response: `MessageResponse` (includes delivery metadata). Duplicate `idempotency_key` returns the existing message.

### Mark Thread As Read
- **PUT** `/v0/chat/threads/{threadId}/read`
- Marks every message not authored by the caller as `read`; responds with HTTP 200 and empty body.

## WebSocket Streaming
- **GET (raw)** `/v0/chat/threads/{threadId}/stream`
- Use the same `Authorization` header as REST calls; the server rejects unauthenticated or non-participant connections.
- On connect, the server pushes up to the 50 most recent messages, then streams new events in <100 ms broadcast windows.
- Every broadcast is sent to all connected participants, including the sender, so mobile clients must dedupe their own outbound messages when rendering.

### Server → Client Payloads
```json
{
  "id": "094e7865-cf4b-4e57-b4cb-7081ff2cd511",
  "thread_id": "6186f9a3-a9a5-43d5-9d1f-f9d5a759357d",
  "sender_id": "<uuid>",
  "content": "All good! I will arrive in 30 minutes.",
  "message_type": "user",
  "status": "sent",
  "sent_at": "2025-11-05T13:06:38.253067Z",
  "delivered_at": null,
  "read_at": null,
  "type": "message"
}
```
- Status updates reuse the same shape with `type: "status"` and updated `delivered_at` / `read_at` timestamps.

### Client ACK (required for delivery/read receipts)
```json
{
  "type": "ack",
  "id": "094e7865-cf4b-4e57-b4cb-7081ff2cd511",
  "status": "read"
}
```
- Valid ack statuses: `delivered`, `read` (transitions must respect `sent → delivered → read`).

### Outbound Messaging
- Persisting messages must go through the HTTP `SendMessage` endpoint so the record hits the database, outbox, and automations.
- WebSocket writes are broadcast-only and should be limited to ACK payloads from mobile clients.

## Automated Lifecycle Hooks
- `booking.assigned` event creates the thread, stores `artisan_user_id`, and sends the welcome status update.
- Subsequent booking status events (`booking.confirmed`, `booking.enroute`, etc.) map to automated system messages defined in `domain/automated_messages.go`.
- Chat events (message sent/delivered/read, thread created) are published via the outbox relay for downstream consumers.

## Local Development
1. Install the Encore CLI and ensure Postgres is available (Encore provisions the service DB when running locally).
2. From `services/protisan`, run `encore run` to start the stack; migrations in `./migrations` apply automatically on first use.
3. The chat service listens on `/v0/chat` within the Encore gateway; authenticate using the same flow as other Protisan services.

## Testing
- Comprehensive end-to-end validation (booking lifecycle + chat):
```bash
RUN_ADVANCED_TESTS=true ./.trench/test-chat-scenarios.sh
```
- Script spins up booking + chat workflows, verifies auth guards, WebSocket latency SLA, ordering, and concurrent message handling.

## Mobile Integration Workflow
1. Wait for the booking assignment push/event to surface the `thread_id`, or list threads via `GET /v0/chat/threads`.
2. Fetch the initial transcript with `GET /v0/chat/threads/{threadId}/messages` (store `idempotency_key` for local dedupe).
3. Open the WebSocket stream at `/v0/chat/threads/{threadId}/stream` with the user’s auth token to receive live messages + system updates.
4. Send user messages through HTTP `POST /v0/chat/threads/{threadId}/messages` with a client-generated `idempotency_key`.
5. When messages render on the device, emit ACK payloads over the WebSocket (`status: delivered` or `read`) to advance server-side receipts.
6. Optionally call `PUT /v0/chat/threads/{threadId}/read` after processing a batch to bulk-mark the thread as read.
