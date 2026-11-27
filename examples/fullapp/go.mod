module github.com/dxrlabs/chassis/examples/fullapp

go 1.26.0

require (
	github.com/dxrlabs/chassis/broker v0.0.0
	github.com/dxrlabs/chassis/broker/nats v0.0.0
	github.com/dxrlabs/chassis/config v0.0.0
	github.com/dxrlabs/chassis/db/postgres v0.0.0
	github.com/dxrlabs/chassis/httpx v0.0.0
	github.com/dxrlabs/chassis/logger v0.0.0
	github.com/dxrlabs/chassis/migrate v0.0.0
	github.com/dxrlabs/chassis/outbox v0.0.0
	github.com/dxrlabs/chassis/redis v0.0.0
	github.com/dxrlabs/chassis/server v0.0.0
	github.com/dxrlabs/chassis/telemetry v0.0.0
	github.com/dxrlabs/chassis/worker v0.0.0
	github.com/jackc/pgx/v5 v5.13.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/caarlos0/env/v11 v11.4.2 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sql-driver/mysql v1.11.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mfridman/interpolate v0.2.0 // indirect
	github.com/nats-io/nats.go v1.56.0 // indirect
	github.com/nats-io/nkeys v0.4.17 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pressly/goose/v3 v3.30.2 // indirect
	github.com/redis/go-redis/v9 v9.24.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.72.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261110000943-09e21b237f2c // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261110000943-09e21b237f2c // indirect
	google.golang.org/grpc v1.85.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	modernc.org/libc v1.80.2 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.63.0 // indirect
)

replace (
	github.com/dxrlabs/chassis/broker => ../../broker
	github.com/dxrlabs/chassis/broker/nats => ../../broker/nats
	github.com/dxrlabs/chassis/config => ../../config
	github.com/dxrlabs/chassis/db/postgres => ../../db/postgres
	github.com/dxrlabs/chassis/httpx => ../../httpx
	github.com/dxrlabs/chassis/logger => ../../logger
	github.com/dxrlabs/chassis/migrate => ../../migrate
	github.com/dxrlabs/chassis/outbox => ../../outbox
	github.com/dxrlabs/chassis/redis => ../../redis
	github.com/dxrlabs/chassis/server => ../../server
	github.com/dxrlabs/chassis/telemetry => ../../telemetry
	github.com/dxrlabs/chassis/worker => ../../worker
)
