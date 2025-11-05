# Quote Service

## Overview
- Encore microservice that manages booking-specific price quotes with full lifecycle transitions (proposed → accepted/rejected/expired/superseded).
- Integrates with booking, artisan, and core services for authorization, status synchronization, and event propagation.
- Persists data in the dedicated `quote` Postgres database (migrations under `./migrations`) while publishing quote events via the shared outbox relay.

## Architecture
- `domain/`: quote aggregate (`Quote`), business service, validator, state machine, and event contracts.
- `handlers/`: HTTP entry points for proposing, viewing, accepting, rejecting, listing, plus cache management.
- `repository/`: GORM-backed repository using optimistic locking, transactional helpers, and outbox writes.
- `events/`: quote-domain event publisher wired into the outbox relay.
- `relay/`: relay runner that pushes queued events to downstream topics.
- `internal/`: auth/authz helpers, logging, config, error normalization, and utility functions.
- Dependencies: Encore runtime, Encore cron, GORM (Postgres), in-memory cache manager, booking/artisan/core service clients.

## Data Model
| Entity | Key fields |
| --- | --- |
| Quote | `id`, `booking_id`, `version`, `state`, `amount_cents`, `currency`, JSON `breakdown`, `notes`, `estimated_duration_mins`, `valid_until`, `proposed_by`, `decision_by`, timestamps, optimistic `db_version`. |

### States & Transitions
- Initial: `proposed` (created via `ProposeQuote`).
- Terminal: `accepted`, `rejected`, `expired`, `superseded` (older proposed quotes auto-superseded when a new version is created).
- Expiration: cron job marks `proposed` quotes as `expired` once `valid_until` has elapsed.

## HTTP APIs (auth required)

### Propose Quote
- **POST** `/v0/quote`
- Only the assigned artisan may call this (validated against booking + artisan service).
- Body:
```json
{
  "booking_id": "<booking_uuid>",
  "amount_cents": 180000,
  "currency": "NGN",
  "estimated_duration_mins": 120,
  "notes": "Includes parts and labor",
  "valid_until": "2025-11-08T15:04:05Z",
  "breakdown": [
    {"label": "Labor", "amount_cents": 80000},
    {"label": "Materials", "amount_cents": 100000}
  ]
}
```
- Response: `QuoteResponse` with newest `version` and state `proposed`; previous non-terminal quotes become `superseded` automatically.

### Get Quote
- **GET** `/v0/quote/item/{quoteId}`
- Returns single quote after auth check (booking customer or assigned artisan).

### List Quotes by Booking
- **GET** `/v0/quote/for-booking/{bookingId}`
- Returns chronological list of all versions for the booking.

### Accept Quote
- **POST** `/v0/quote/{quoteId}/accept`
- Body: `{ "booking_id": "<booking_uuid>" }`
- Only the booking customer can accept; transitions state to `accepted` and emits event for payment pipeline.

### Reject Quote
- **POST** `/v0/quote/{quoteId}/reject`
- Body:
```json
{
  "booking_id": "<booking_uuid>",
  "reason_code": "price_too_high",
  "reason_text": "Need a more competitive offer"
}
```
- Only the booking customer can reject; valid `reason_code` values defined in `domain/state.go` (e.g. `price_too_high`, `timeline_mismatch`, `found_alternative`, `no_longer_needed`, `other`).

## Scheduled Tasks
- Encore cron job `quote-expiry` executes `POST /internal/quote/expire` every 5 minutes, marking lapsed `proposed` quotes as `expired` and pushing domain events.

## Caching & Outbox
- Read-heavy endpoints cache quote documents (`quote:{id}`) and booking lists (`quotes:booking:{bookingId}`) in the in-memory cache; all mutations purge affected keys.
- Each state transition enqueues a `QuoteEvent` in the outbox table; `relay/` consumes and publishes to the core relay for other services.

## Testing
- End-to-end validation script (booking lifecycle + three quote revisions):
```bash
RUN_ADVANCED_TESTS=true ./.trench/test-quote-scenarios.sh
```
- Script provisions booking, exercises propose/reject/accept flows, and verifies booking status transitions plus quote state changes.

## Mobile Integration Workflow
1. **Artisan app** retrieves eligible bookings, then calls `POST /v0/quote` with line-item `breakdown` to publish a quote (supply client-side ISO `valid_until` when needed).
2. **Customer app** polls `GET /v0/quote/for-booking/{bookingId}` or refetches individual quotes to display the latest `proposed` version; cached responses will refresh automatically after mutations.
3. Customer accepts via `POST /v0/quote/{quoteId}/accept` or rejects with `POST /v0/quote/{quoteId}/reject` and a standardized `reason_code`; the server updates booking status and emits events consumed by payment/chat services.
4. After acceptance, downstream services react through the outbox relay—clients should refresh booking state via the bookings API to confirm the `quote_accepted` status.
