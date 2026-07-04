SHELL := /bin/bash

ADDR ?= 127.0.0.1:8080
PRINTER ?= 192.168.86.22:9100
TEMPLATES_DIR ?= templates
STATE_FILE ?= printer_state.json
BIN ?= /tmp/epson-pos-dashboard
LOG ?= /tmp/epson-pos-dashboard.log
PIDFILE ?= /tmp/epson-pos-dashboard.pid
PORT := $(lastword $(subst :, ,$(ADDR)))

.DEFAULT_GOAL := default

.PHONY: default build start stop restart status logs is-running

default:
	@if $(MAKE) -s is-running >/dev/null 2>&1; then \
		echo "server is running on $(ADDR); restarting"; \
		$(MAKE) -s restart; \
	else \
		$(MAKE) -s start; \
	fi

build:
	@go build -o "$(BIN)" .

start: build
	@if $(MAKE) -s is-running >/dev/null 2>&1; then \
		echo "server already running on $(ADDR)"; \
		exit 0; \
	fi
	@mkdir -p "$(dir $(LOG))" "$(dir $(PIDFILE))"
	@nohup "$(BIN)" serve --addr "$(ADDR)" --printer "$(PRINTER)" --templates-dir "$(TEMPLATES_DIR)" --state-file "$(STATE_FILE)" >"$(LOG)" 2>&1 & echo $$! >"$(PIDFILE)"
	@sleep 1
	@if $(MAKE) -s is-running >/dev/null 2>&1; then \
		echo "server started: http://$(ADDR)/"; \
		echo "printer: $(PRINTER)"; \
		echo "templates: $(TEMPLATES_DIR)"; \
		echo "state: $(STATE_FILE)"; \
		echo "log: $(LOG)"; \
	else \
		echo "server failed to start; log follows:"; \
		tail -n 40 "$(LOG)" 2>/dev/null || true; \
		exit 1; \
	fi

stop:
	@stopped=0; \
	if [ -f "$(PIDFILE)" ]; then \
		pid="$$(cat "$(PIDFILE)")"; \
		if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then \
			kill "$$pid" 2>/dev/null || true; \
			stopped=1; \
		fi; \
		rm -f "$(PIDFILE)"; \
	fi; \
	for pid in $$(lsof -tiTCP:$(PORT) -sTCP:LISTEN 2>/dev/null); do \
		kill "$$pid" 2>/dev/null || true; \
		stopped=1; \
	done; \
	for _ in 1 2 3 4 5; do \
		if ! lsof -tiTCP:$(PORT) -sTCP:LISTEN >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 0.2; \
	done; \
	for pid in $$(lsof -tiTCP:$(PORT) -sTCP:LISTEN 2>/dev/null); do \
		kill -9 "$$pid" 2>/dev/null || true; \
		stopped=1; \
	done; \
	if [ "$$stopped" = "1" ]; then \
		echo "server stopped on $(ADDR)"; \
	else \
		echo "server not running on $(ADDR)"; \
	fi

restart: stop start

status:
	@if $(MAKE) -s is-running >/dev/null 2>&1; then \
		echo "server running on $(ADDR)"; \
		lsof -nP -iTCP:$(PORT) -sTCP:LISTEN 2>/dev/null || true; \
	else \
		echo "server not running on $(ADDR)"; \
	fi

logs:
	@tail -f "$(LOG)"

is-running:
	@if [ -f "$(PIDFILE)" ] && kill -0 "$$(cat "$(PIDFILE)")" 2>/dev/null; then \
		exit 0; \
	fi
	@lsof -tiTCP:$(PORT) -sTCP:LISTEN >/dev/null 2>&1
