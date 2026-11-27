# chassis

Modular, production-ready Go infrastructure for HTTP services. Each package is an independently versioned Go module with a uniform lifecycle: `New(...) (client, cleanup func(), error)` so `main` can orchestrate LIFO shutdown with `defer`.

## Decisions / production

- [`docs/decisions.md`](docs/decisions.md) — migrations, telemetry, versioning
- [`docs/production.md`](docs/production.md) — runtime contracts and deploy checklist

## Modules

| Module | Import path | Role |
|--------|-------------|------|
| `config` | `github.com/dxrlabs/chassis/config` | Typed env loading |
| `logger` | `github.com/dxrlabs/chassis/logger` | `slog` JSON/text |
| `telemetry` | `github.com/dxrlabs/chassis/telemetry` | OTLP + HTTP middleware + slog trace IDs |
| `db/postgres` | `github.com/dxrlabs/chassis/db/postgres` | pgx pool |
| `db/mysql` | `github.com/dxrlabs/chassis/db/mysql` | database/sql MySQL |
| `db/sqlite` | `github.com/dxrlabs/chassis/db/sqlite` | modernc SQLite |
| `db/spanner` | `github.com/dxrlabs/chassis/db/spanner` | Cloud Spanner client |
| `redis` | `github.com/dxrlabs/chassis/redis` | go-redis (cache alongside a DB) |
| `migrate` | `github.com/dxrlabs/chassis/migrate` | goose + raw SQL (postgres/mysql/sqlite) |
| `migrate/spanner` | `github.com/dxrlabs/chassis/migrate/spanner` | Spanner Admin DDL |
| `outbox` | `github.com/dxrlabs/chassis/outbox` | Transactional outbox + inbox idempotency |
| `broker` | `github.com/dxrlabs/chassis/broker` | `Bus` contracts |
| `broker/nats` | `…/broker/nats` | NATS driver |
| `broker/rabbitmq` | `…/broker/rabbitmq` | RabbitMQ driver |
| `broker/kafka` | `…/broker/kafka` | Kafka driver |
| `broker/pubsub` | `…/broker/pubsub` | GCP Pub/Sub driver |
| `worker` | `github.com/dxrlabs/chassis/worker` | Background / periodic jobs |
| `server` | `github.com/dxrlabs/chassis/server` | JSON `/healthz` `/readyz`, drain shutdown |
| `httpx` | `github.com/dxrlabs/chassis/httpx` | JSON helpers, Bearer auth, middleware |

## Local development

```bash
make ci          # work sync, fmt, vet, test, build
make test-race
```

## Versioning (GitHub tags)

[Release Please](.github/workflows/release-please.yml) opens release PRs and creates **per-module** tags on merge:

```text
db/postgres/v1.2.0
broker/nats/v1.0.1
outbox/v1.0.0
```

Breaking changes in one module bump **that module’s** major only. Conventional commits on the release PR drive the bump.

```bash
go get github.com/dxrlabs/chassis/db/postgres@v1.2.0
```

## Examples

| Example | Stack |
|---------|--------|
| [`examples/fullapp`](examples/fullapp) | Postgres + Redis + NATS + migrate + server |
| [`examples/sqliteapp`](examples/sqliteapp) | SQLite + outbox (tx write) + server — no external deps |

## Outbox / inbox

1. Apply `outbox/schema/{postgres,mysql,sqlite}.sql` (or embed in app migrations).
2. `InsertOutboxTx` in the same DB transaction as business writes.
3. `outbox.NewRelay(store, bus, log)` publishes pending rows.
4. Consumers: `outbox.InboxHandler(store, idFn, handler)` for idempotency.
