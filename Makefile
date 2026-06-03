# Root Makefile for 1agents project
#
# Provides a unified build, package, and deployment workflow for both Linux and macOS.

APP          := 1agents
VERSION      := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT       := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME   := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
HOSTNAME     := $(shell hostname 2>/dev/null || uname -n 2>/dev/null || echo "unknown")
OS           := $(shell uname -s | tr '[:upper:]' '[:lower:]' 2>/dev/null || echo "unknown")
ARCH         := $(shell uname -m 2>/dev/null || echo "unknown")

# Lowercase OS and ARCH for filename consistency
OS_LOWER     := $(shell echo $(OS) | tr '[:upper:]' '[:lower:]')
ARCH_LOWER   := $(shell echo $(ARCH) | tr '[:upper:]' '[:lower:]')

# Go LDFLAGS for injecting version, commit (including host details) and build time
AGENT_LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME) \
  -X main.buildTime=$(BUILD_TIME)

.PHONY: all frontend ttyd cc-connect cc-connect-noweb backend agent package clean help

help:
	@echo "Unified Build and Packaging System for Remote Agents"
	@echo "Host: $(HOSTNAME) ($(OS)/$(ARCH))"
	@echo ""
	@echo "Available targets:"
	@echo "  make all               - Build all components (frontend, ttyd, cc-connect, backend)"
	@echo "  make frontend          - Build frontend assets (html/) and generate modules/ttyd/src/html.h"
	@echo "  make ttyd              - Compile native ttyd C server natively on the current host"
	@echo "  make cc-connect        - Compile cc-connect Go daemon (with web assets)"
	@echo "  make cc-connect-noweb  - Compile cc-connect Go daemon (WITHOUT rebuilding web assets)"
	@echo "  make backend           - Compile 1agents Go server (backend) with version ldflags"
	@echo "  make package           - Create a target-distinguished deployable archive in dist/"
	@echo "  make clean             - Clean all intermediate and build outputs across components"

all: frontend ttyd cc-connect backend

frontend:
	@echo "=== Building Frontend (html/)..."
	cd html && corepack enable && yarn install && yarn build

ttyd:
	@echo "=== Building ttyd terminal server..."
	@if [ "$(OS)" = "Darwin" ]; then \
		cmake -DCMAKE_PREFIX_PATH="/opt/homebrew;/usr/local" -DCMAKE_BUILD_TYPE=Release -B build-ttyd -S modules/ttyd ; \
	else \
		cmake -DCMAKE_BUILD_TYPE=Release -B build-ttyd -S modules/ttyd ; \
	fi
	make -C build-ttyd
	@mkdir -p build
	cp build-ttyd/ttyd build/ttyd

cc-connect:
	@echo "=== Building cc-connect daemon..."
	$(MAKE) -C modules/cc-connect build
	@mkdir -p build
	cp modules/cc-connect/cc-connect build/cc-connect

cc-connect-noweb:
	@echo "=== Building cc-connect daemon (no web build)..."
	$(MAKE) -C modules/cc-connect build-noweb
	@mkdir -p build
	cp modules/cc-connect/cc-connect build/cc-connect

backend:
	@echo "=== Building 1agents Go server (backend)..."
	mkdir -p build
	cd backend && go build -ldflags "$(AGENT_LDFLAGS)" -o ../build/1agents ./cmd/backend

agent: backend

package: all
	@echo "=== Packaging 1agents for $(OS_LOWER)-$(ARCH_LOWER) on $(HOSTNAME)..."
	@rm -rf dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	@mkdir -p dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin
	cp build/1agents dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/ttyd dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/cc-connect dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp -r html/dist dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/dist
	cd dist && tar -czf 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	@echo "=== Created package: dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz"

clean:
	@echo "=== Cleaning build artifacts..."
	rm -rf build build-ttyd dist
	rm -rf html/dist modules/ttyd/src/html.h
	$(MAKE) -C modules/cc-connect clean
	rm -rf src-tauri/resources src-tauri/target

.PHONY: tauri-resources tauri-dev tauri-build

tauri-resources: all
	@echo "=== Setting up Tauri resources ==="
	./scripts/setup-resources.sh

tauri-dev: tauri-resources
	@echo "=== Starting Tauri in development mode ==="
	npx @tauri-apps/cli dev

tauri-build: tauri-resources
	@echo "=== Building Tauri production bundle ==="
	npx @tauri-apps/cli build
