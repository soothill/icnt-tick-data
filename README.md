<!-- Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com) -->

# ICNT Tick Data Pipeline

This repository collects ICNT/USD tick data from Kraken Pro’s public trade feed, caches it locally, aggregates minute bars, and reliably ships both raw ticks and minute summaries to InfluxDB. It is designed for openSUSE MicroOS using Podman + systemd (Quadlet).

## Features

- WebSocket ingest with REST backfill to close gaps.
- SQLite cache to protect against data loss and support replays.
- Minute bar aggregation that refreshes existing bars when late ticks arrive.
- Batched InfluxDB writes with retry/backoff and clear error reporting.

## How It Works

- **Ingest**: WebSocket stream from Kraken for real-time trades.
- **Backfill**: REST polling to fill gaps after disconnects.
- **Cache**: SQLite database on local disk to avoid data loss.
- **Aggregate**: Minute bars built from cached ticks (open/high/low/close/volume).
- **Flush**: Batched writes to InfluxDB with retry/backoff if the DB is down.

## Project Layout

- `cmd/pipeline/` - Go entrypoint.
- `internal/pipeline/` - Kraken clients, cache, aggregation, Influx writer.
- `systemd/` - Quadlet unit files for Podman.
- `config/` - Example env files (templates only).

## Requirements

- Go 1.21+
- Podman + systemd (for the containerized flow)

## Quick Start (MicroOS + Podman)

1) Build the image:

```sh
make build
```

2) Install units and default configs:

```sh
make install
```

3) Edit the real config file (outside repo):

- `~/.config/icnt-tick-data/icnt-tick-data.env` (includes pipeline and InfluxDB init settings)

4) Enable and start containers:

```sh
make enable
make start
```

5) Check status/logs:

```sh
make status
make logs-tail
```

6) Validate connectivity (uses your env config):

```sh
make check-connectivity
```

7) Run tests in Podman:

```sh
make test
```

## Configuration

Pipeline + Influx setup (`icnt-tick-data.env`):

- `KRAKEN_PAIR` (default `ICNT/USD`)
- `KRAKEN_REST_PAIR` (default `ICNTUSD`)
- `CACHE_DB_PATH` (default `/data/ticks.sqlite` in container)
- `INFLUX_URL`, `INFLUX_ORG`, `INFLUX_BUCKET`, `INFLUX_TOKEN`
- `FLUSH_INTERVAL_SEC`, `BACKFILL_INTERVAL_SEC`, `AGGREGATE_INTERVAL_SEC`, `BATCH_SIZE`
- `DOCKER_INFLUXDB_INIT_*` values for initial InfluxDB org/bucket/token setup

## Security Notes

- Do not commit real credentials. Only `config/*.env.example` belongs in git.
- Real configs live under `~/.config/icnt-tick-data/` and are ignored.

## Local Development

Run the pipeline directly:

```sh
go run ./cmd/pipeline
```

Export the same env variables in your shell or a local `.env` file.

Run tests:

```sh
go test ./...
```
