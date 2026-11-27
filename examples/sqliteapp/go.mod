module github.com/dxrlabs/chassis/examples/sqliteapp

go 1.26.0

require (
	github.com/dxrlabs/chassis/config v0.0.0
	github.com/dxrlabs/chassis/db/sqlite v0.0.0
	github.com/dxrlabs/chassis/httpx v0.0.0
	github.com/dxrlabs/chassis/logger v0.0.0
	github.com/dxrlabs/chassis/migrate v0.0.0
	github.com/dxrlabs/chassis/outbox v0.0.0
	github.com/dxrlabs/chassis/server v0.0.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/caarlos0/env/v11 v11.4.2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/dxrlabs/chassis/broker v0.0.0 // indirect
	github.com/go-sql-driver/mysql v1.11.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.13.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mfridman/interpolate v0.2.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pressly/goose/v3 v3.30.2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	modernc.org/libc v1.80.2 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.63.0 // indirect
)

replace (
	github.com/dxrlabs/chassis/broker => ../../broker
	github.com/dxrlabs/chassis/config => ../../config
	github.com/dxrlabs/chassis/db/sqlite => ../../db/sqlite
	github.com/dxrlabs/chassis/httpx => ../../httpx
	github.com/dxrlabs/chassis/logger => ../../logger
	github.com/dxrlabs/chassis/migrate => ../../migrate
	github.com/dxrlabs/chassis/outbox => ../../outbox
	github.com/dxrlabs/chassis/server => ../../server
	github.com/dxrlabs/chassis/worker => ../../worker
)
