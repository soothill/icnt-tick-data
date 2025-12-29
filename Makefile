# Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

SHELL := /bin/bash

.DEFAULT_GOAL := help

IMAGE_NAME ?= localhost/icnt-tick-data:latest
TEST_IMAGE ?= docker.io/library/golang:1.21

.PHONY: help build install install-config install-units reload enable start stop restart status logs logs-tail test network clean

QUADLET_DIR := $(HOME)/.config/containers/systemd
CONFIG_DIR := $(HOME)/.config/icnt-tick-data
SYSTEMCTL := systemctl --user

help: ## Show available make targets and their descriptions
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Podman image locally
	podman build -t $(IMAGE_NAME) -f Containerfile .

network: ## Ensure the icnt-tick-data Podman network exists
	podman network inspect icnt-tick-data >/dev/null 2>&1 || podman network create icnt-tick-data

install-config: ## Copy example env files to the local config directory if missing
	mkdir -p $(CONFIG_DIR)
	if [ ! -f $(CONFIG_DIR)/icnt-tick-data.env ]; then \
		cp config/icnt-tick-data.env.example $(CONFIG_DIR)/icnt-tick-data.env; \
	fi
	if [ ! -f $(CONFIG_DIR)/influxdb.env ]; then \
		cp config/influxdb.env.example $(CONFIG_DIR)/influxdb.env; \
	fi

install-units: ## Install Quadlet unit files into the user systemd directory
	mkdir -p $(QUADLET_DIR)
	cp systemd/icnt-tick-data.container $(QUADLET_DIR)/
	cp systemd/influxdb.container $(QUADLET_DIR)/

install: network install-config install-units reload ## Install config, units, and reload systemd

reload: ## Reload the user systemd daemon
	$(SYSTEMCTL) daemon-reload

enable: ## Enable the container services for the current user
	$(SYSTEMCTL) enable container-influxdb.service
	$(SYSTEMCTL) enable container-icnt-tick-data.service

start: ## Start the container services
	$(SYSTEMCTL) start container-influxdb.service
	$(SYSTEMCTL) start container-icnt-tick-data.service

stop: ## Stop the container services
	-$(SYSTEMCTL) stop container-icnt-tick-data.service
	-$(SYSTEMCTL) stop container-influxdb.service

restart: ## Restart the container services
	$(SYSTEMCTL) restart container-icnt-tick-data.service
	$(SYSTEMCTL) restart container-influxdb.service

status: ## Show status for the tick data service
	$(SYSTEMCTL) status container-icnt-tick-data.service

logs: ## Show status (with logs) for both services
	$(SYSTEMCTL) status container-icnt-tick-data.service --no-pager
	$(SYSTEMCTL) status container-influxdb.service --no-pager

logs-tail: ## Tail logs for both services
	journalctl --user -u container-icnt-tick-data.service -u container-influxdb.service -f

test: ## Run Go tests inside a Podman container
	podman run --rm -v $(PWD):/src:Z -w /src $(TEST_IMAGE) go test ./...

clean: ## Remove the built image
	podman image rm -f $(IMAGE_NAME)
