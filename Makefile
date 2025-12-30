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
	KRAKEN_WS_HOST=$$(printf "%s" "$$KRAKEN_WS_URL" | sed -E 's#^[a-zA-Z]+://([^/:]+).*$#\\1#'); \
	KRAKEN_WS_PORT=$$(printf "%s" "$$KRAKEN_WS_URL" | sed -nE 's#^[a-zA-Z]+://[^/:]+:([0-9]+).*$#\\1#p'); \
	if [ -z "$$KRAKEN_WS_PORT" ]; then \
		if printf "%s" "$$KRAKEN_WS_URL" | grep -qi '^wss://'; then KRAKEN_WS_PORT=443; else KRAKEN_WS_PORT=80; fi; \
	fi; \
	fail=0; \
	if printf "%s" "$$INFLUX_DISABLED" | grep -qi '^\(1\|true\|yes\)$'; then \
		echo "[skip] InfluxDB checks disabled by INFLUX_DISABLED"; \
	else \
		echo "Checking InfluxDB health at $$INFLUX_URL/health"; \
		if curl -fsS --max-time 5 "$$INFLUX_URL/health" >/dev/null; then \
			echo "[ok] InfluxDB health"; \
		else \
			echo "[fail] InfluxDB health"; fail=1; \
		fi; \
	fi; \
	echo "Checking Kraken REST at $$KRAKEN_REST_URL?pair=$$KRAKEN_REST_PAIR"; \
	if curl -fsS --max-time 8 "$$KRAKEN_REST_URL?pair=$$KRAKEN_REST_PAIR" >/dev/null; then \
		echo "[ok] Kraken REST"; \
	else \
		echo "[fail] Kraken REST"; fail=1; \
	fi; \
	echo "Checking Kraken WS TCP $$KRAKEN_WS_HOST:$$KRAKEN_WS_PORT"; \
	WS_PROBE="cat </dev/null >/dev/tcp/$$KRAKEN_WS_HOST/$$KRAKEN_WS_PORT"; \
	if timeout 5 bash -c "$$WS_PROBE" >/dev/null 2>&1; then \
		echo "[ok] Kraken WS TCP reachable"; \
	else \
		echo "[fail] Kraken WS TCP reachable"; fail=1; \
	fi; \
	exit $$fail

clean: ## Remove the built image
	podman image rm -f $(IMAGE_NAME)
