# Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

SHELL := /bin/bash

IMAGE_NAME ?= localhost/icnt-tick-data:latest
TEST_IMAGE ?= docker.io/library/golang:1.21

QUADLET_DIR := $(HOME)/.config/containers/systemd
CONFIG_DIR := $(HOME)/.config/icnt-tick-data
SYSTEMCTL := systemctl --user

.PHONY: build install install-config install-units reload enable start stop restart status logs logs-tail test network clean

build:
	podman build -t $(IMAGE_NAME) -f Containerfile .

network:
	podman network inspect icnt-tick-data >/dev/null 2>&1 || podman network create icnt-tick-data

install-config:
	mkdir -p $(CONFIG_DIR)
	if [ ! -f $(CONFIG_DIR)/icnt-tick-data.env ]; then \
		cp config/icnt-tick-data.env.example $(CONFIG_DIR)/icnt-tick-data.env; \
	fi
	if [ ! -f $(CONFIG_DIR)/influxdb.env ]; then \
		cp config/influxdb.env.example $(CONFIG_DIR)/influxdb.env; \
	fi

install-units:
	mkdir -p $(QUADLET_DIR)
	cp systemd/icnt-tick-data.container $(QUADLET_DIR)/
	cp systemd/influxdb.container $(QUADLET_DIR)/

install: network install-config install-units reload

reload:
	$(SYSTEMCTL) daemon-reload

enable:
	$(SYSTEMCTL) enable container-influxdb.service
	$(SYSTEMCTL) enable container-icnt-tick-data.service

start:
	$(SYSTEMCTL) start container-influxdb.service
	$(SYSTEMCTL) start container-icnt-tick-data.service

stop:
	-$(SYSTEMCTL) stop container-icnt-tick-data.service
	-$(SYSTEMCTL) stop container-influxdb.service

restart:
	$(SYSTEMCTL) restart container-icnt-tick-data.service
	$(SYSTEMCTL) restart container-influxdb.service

status:
	$(SYSTEMCTL) status container-icnt-tick-data.service

logs:
	$(SYSTEMCTL) status container-icnt-tick-data.service --no-pager
	$(SYSTEMCTL) status container-influxdb.service --no-pager

logs-tail:
	journalctl --user -u container-icnt-tick-data.service -u container-influxdb.service -f

test:
	podman run --rm -v $(PWD):/src:Z -w /src $(TEST_IMAGE) go test ./...

clean:
	podman image rm -f $(IMAGE_NAME)
