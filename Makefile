# Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

SHELL := /bin/bash

.DEFAULT_GOAL := help

IMAGE_NAME ?= localhost/icnt-tick-data:latest
TEST_IMAGE ?= docker.io/library/golang:1.21
BIN ?= pipeline
ENV_FILE ?= $(CONFIG_DIR)/icnt-tick-data.env

.PHONY: help build build-local run install install-config install-units reload enable start stop restart status logs logs-tail test test-local fmt tidy network clean check-connectivity

QUADLET_DIR := $(HOME)/.config/containers/systemd
CONFIG_DIR := $(HOME)/.config/icnt-tick-data
SYSTEMCTL := systemctl --user

help: ## Show available make targets and their descriptions
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Podman image locally
	podman build -t $(IMAGE_NAME) -f Containerfile .

build-local: ## Build the pipeline binary locally (non-container)
	go build -o $(BIN) ./cmd/pipeline

run: ## Run the pipeline locally (loads ENV_FILE if present)
	if [ -f $(ENV_FILE) ]; then \
		set -a; source $(ENV_FILE); set +a; \
	fi; \
	go run ./cmd/pipeline

network: ## Ensure the icnt-tick-data Podman network exists
	podman network inspect icnt-tick-data >/dev/null 2>&1 || podman network create icnt-tick-data

install-config: ## Copy example env files to the local config directory if missing
	mkdir -p $(CONFIG_DIR)
	if [ ! -f $(CONFIG_DIR)/icnt-tick-data.env ]; then \
		cp config/icnt-tick-data.env.example $(CONFIG_DIR)/icnt-tick-data.env; \
	fi

install-units: ## Install Quadlet unit files into the user systemd directory
	mkdir -p $(QUADLET_DIR)
	cp systemd/icnt-tick-data.container $(QUADLET_DIR)/

install: network install-config install-units reload ## Install config, units, and reload systemd

reload: ## Reload the user systemd daemon
	$(SYSTEMCTL) daemon-reload

enable: install ## Enable the container services for the current user (ensures units installed)
	$(SYSTEMCTL) enable icnt-tick-data.service

start: ## Start the container services
	$(SYSTEMCTL) start icnt-tick-data.service

stop: ## Stop the container services
	-$(SYSTEMCTL) stop icnt-tick-data.service

restart: ## Restart the container services
	$(SYSTEMCTL) restart icnt-tick-data.service

status: ## Show status for the tick data service
	$(SYSTEMCTL) status icnt-tick-data.service

logs: ## Show status (with logs) for both services
	$(SYSTEMCTL) status icnt-tick-data.service --no-pager

logs-tail: ## Tail logs for both services
	journalctl --user -u icnt-tick-data.service -f

test: ## Run Go tests inside a Podman container
	podman run --rm -v $(PWD):/src:Z -w /src $(TEST_IMAGE) go test ./...

test-local: ## Run Go tests on the host
	go test ./...

fmt: ## Format Go code
	gofmt -w cmd internal

tidy: ## Sync module dependencies
	@if command -v go >/dev/null 2>&1; then \
		go mod tidy; \
	elif command -v podman >/dev/null 2>&1; then \
		podman run --rm -v $(PWD):/src:Z -w /src $(TEST_IMAGE) go mod tidy; \
	else \
		echo "go or podman not found; install Go 1.21+ or Podman to run tidy" >&2; \
		exit 1; \
	fi

check-connectivity: ## Verify connectivity to InfluxDB and Kraken endpoints using current config
	@if [ -f $(ENV_FILE) ]; then set -a; source $(ENV_FILE); set +a; fi; \
	INFLUX_DISABLED=$${INFLUX_DISABLED:-0}; \
	INFLUX_URL=$${INFLUX_URL:-http://localhost:8086}; \
	KRAKEN_REST_URL=$${KRAKEN_REST_URL:-https://api.kraken.com/0/public/Trades}; \
	KRAKEN_REST_PAIR=$${KRAKEN_REST_PAIR:-$${KRAKEN_PAIR:-ICNT/USD}}; \
	KRAKEN_REST_PAIR=$${KRAKEN_REST_PAIR//[\/-]/}; \
	KRAKEN_WS_URL=$${KRAKEN_WS_URL:-wss://ws.kraken.com}; \
	if command -v python3 >/dev/null 2>&1; then PY_CMD=python3; \
	elif command -v python >/dev/null 2>&1; then PY_CMD=python; \
	elif command -v podman >/dev/null 2>&1; then PY_CMD="podman run --rm -e INFLUX_DISABLED -e INFLUX_URL -e KRAKEN_REST_URL -e KRAKEN_REST_PAIR -e KRAKEN_WS_URL python:3.11-slim python"; \
	else echo "python (or python3) not found; install Python 3 or Podman to run check" >&2; exit 1; fi; \
	$$PY_CMD - <<'PY'
import os
import socket
import ssl
		import sys
		import urllib.parse
		import urllib.request


		def bool_env(val: str) -> bool:
		    return str(val).strip().lower() in ("1", "true", "yes", "on")


		influx_disabled = bool_env(os.environ.get("INFLUX_DISABLED", "0"))
		influx_url = os.environ["INFLUX_URL"].rstrip("/")
		kraken_rest_url = os.environ["KRAKEN_REST_URL"]
		kraken_rest_pair = os.environ["KRAKEN_REST_PAIR"]
		kraken_ws_url = os.environ["KRAKEN_WS_URL"]
		failures = []


		def check_http(name, url, params=None, headers=None):
		    final = url
		    if params:
		        final = url + ("&" if "?" in url else "?") + urllib.parse.urlencode(params)
		    try:
		        req = urllib.request.Request(final, headers=headers or {})
		        with urllib.request.urlopen(req, timeout=8) as resp:
		            sys.stdout.write(f"[ok] {name} -> {resp.status}\\n")
		            return True
		    except Exception as exc:
		        sys.stdout.write(f"[fail] {name}: {exc}\\n")
		        return False


		def check_ws(endpoint):
		    parsed = urllib.parse.urlparse(endpoint)
		    host = parsed.hostname
		    port = parsed.port or (443 if parsed.scheme == "wss" else 80)
		    try:
		        sock = socket.create_connection((host, port), timeout=5)
		        if parsed.scheme == "wss":
		            ctx = ssl.create_default_context()
		            sock = ctx.wrap_socket(sock, server_hostname=host)
		        sock.close()
		        sys.stdout.write(f"[ok] {endpoint} reachable\\n")
		        return True
		    except Exception as exc:
		        sys.stdout.write(f"[fail] {endpoint}: {exc}\\n")
		        return False


		if not influx_disabled:
		    health = f"{influx_url}/health"
		    if not check_http("InfluxDB health", health):
		        failures.append("influx")
		else:
		    sys.stdout.write("[skip] InfluxDB checks disabled by INFLUX_DISABLED\\n")

		rest_params = {"pair": kraken_rest_pair}
		if not check_http("Kraken REST", kraken_rest_url, rest_params):
		    failures.append("kraken_rest")

		if not check_ws(kraken_ws_url):
		    failures.append("kraken_ws")

	sys.exit(1 if failures else 0)
	PY

clean: ## Remove the built image
	podman image rm -f $(IMAGE_NAME)
