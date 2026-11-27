# Decisions

## Migrations: goose + raw SQL (not a new DSL)

**Choice:** Keep [goose](https://github.com/pressly/goose) applying **raw `.sql` files** embedded with `//go:embed`.

Goose does not replace SQL — it versions and runs the SQL you write. That is the right split for a domain-agnostic chassis: apps own schema, chassis owns apply/order/lock.

| Engine | Module | Notes |
|--------|--------|-------|
| Postgres / MySQL / SQLite | `migrate` | `Up(..., Dialect)` — goose dialects |
| Cloud Spanner | `migrate/spanner` | Admin API DDL statements (goose is a poor fit) |

Do **not** introduce a custom migration language. Atlas/Liquibase stay optional at the org level if you need cross-store policy later.

## Telemetry depth: providers + opt-in HTTP/log correlation

**Choice:** Extend `telemetry` only:

1. OTLP TracerProvider / MeterProvider (existing)
2. `HTTPMiddleware` via `otelhttp` — apps wrap their mux in `main`
3. `WrapLogger` / slog handler attrs for `trace_id` / `span_id`

**Do not** import OTel from `server`, `db/*`, or `httpx`. That preserves zero cross-module coupling; apps compose instrumentation at the composition root.

DB/driver spans (pgx, redis) can be added later as optional thin wrappers or left to app-level otel plugins.

## Versioning: Release Please → path tags

Each module is tagged independently as `{module}/vX.Y.Z` (e.g. `db/postgres/v1.2.0`, `broker/nats/v1.0.0`) via GitHub Actions + Release Please. Breaking API changes bump the major for that module only.
