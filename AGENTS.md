<!-- Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com) -->

# Repository Guidelines

## Project Structure & Module Organization

This repository contains a Go pipeline for ingesting ICNT/USD tick data. Current layout:

- `cmd/pipeline/` for the pipeline entrypoint (`main.go`).
- `internal/pipeline/` for pipeline modules (config, storage, Kraken clients, Influx writer).
- `systemd/` for Podman Quadlet unit files.
- `config/` for env file templates.
- `cache/` for the local SQLite cache (ignored by git).
- Tests live alongside packages (`*_test.go`) or in `tests/` if you add a separate suite.

If you choose a different structure, update this file so new contributors can orient quickly.

## Build, Test, and Development Commands

There are no build or test scripts defined yet. For local runs:

- `go run ./cmd/pipeline` - start the ingest pipeline.
- `go test ./...` - run all tests (once added).
- `make build` - build the Podman image locally.
- `make install` - install Quadlet units and default env files.
- `make start` - start the InfluxDB and pipeline containers.
- `make enable` - enable the containers on login.
- `make restart` - restart the containers.
- `make status` - view unit status.
- `make logs-tail` - follow logs from both containers.
- `make test` - run Go tests in a Podman container.

## Coding Style & Naming Conventions

No language-specific style rules exist yet. Until a formatter is chosen, keep changes small and consistent within each file. Recommended defaults:

- Indentation: 2 spaces for JSON/YAML; Go code uses `gofmt` (tabs) by default.
- Naming: `snake_case` for files, `PascalCase` for exported types, `camelCase` for variables.
- Go formatting: `gofmt` on save or before commit; linting via `golangci-lint` if added.

## Testing Guidelines

No testing framework is configured. If you add tests:

- Place unit tests in `tests/` with names like `test_<module>.py` or `<module>.spec.ts`.
- Document how to run fast vs. full test suites.
- State any coverage expectations (e.g., “new code should include tests”).

## Commit & Pull Request Guidelines

This repo has no commit history yet. Use clear, imperative commit subjects until a convention is adopted, for example:

- `Add tick data loader`
- `Fix CSV parsing for empty rows`

For pull requests, include:

- A short summary of changes and rationale.
- Links to related issues or data samples.
- Screenshots or sample outputs when behavior changes.

## Security & Configuration Tips

Do not commit secrets or credentials. Real env files live outside the repo at `~/.config/icnt-tick-data/` and are ignored by git; only `config/*.env.example` templates should be committed. Document required environment variables in `README.md` once they exist.
