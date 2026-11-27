# Production readiness

## Runtime contracts

| Concern | Behavior |
|---------|----------|
| Lifecycle | Every stateful module: `New → (client, cleanup, error)`; LIFO `defer` in `main` |
| HTTP | JSON only for chassis endpoints (`/healthz`, `/readyz`, errors, timeouts) |
| Shutdown | Server drains in-flight requests; `/readyz` flips to `not_ready` while draining |
| Outbox | Lease + `SKIP LOCKED` (Postgres/MySQL) or serialized tx (SQLite); **at-least-once** publish |
| Inbox | Idempotent consumers via `InboxHandler` |
| Telemetry | Opt-in: wrap mux with `telemetry.HTTPMiddleware`; `WrapLogger` for trace IDs |
| Versioning | Release Please tags `module/vX.Y.Z` independently |

## Outbox multi-instance

1. Claim sets `locked_until` and increments `attempts`.
2. Concurrent relays skip locked rows (`FOR UPDATE SKIP LOCKED`).
3. Publish failure leaves the lease; after expiry another relay retries.
4. Consumers **must** use inbox (or equivalent) — duplicates are possible after publish+crash before `MarkPublished`.

## Checklist before first prod deploy

- [ ] Merge to default branch; Release Please opened/merged for initial tags
- [ ] Pin modules in apps (`go get …@vX.Y.Z`), no `replace` outside the monorepo
- [ ] Apply outbox schema (or app migrations that include it)
- [ ] Wire `NewRelay` + `InboxHandler` for any bus publish path
- [ ] Set `OTLP_ENDPOINT` in prod; leave empty only for local
- [ ] Readiness checks cover primary DB (+ Redis if required for traffic)
- [ ] Run `make ci` and `make test-race` green
