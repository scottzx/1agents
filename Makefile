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

.PHONY: all frontend ttyd cc-connect cc-connect-noweb cc-switch backend agent package clean help

help:
	@echo "Unified Build and Packaging System for Remote Agents"
	@echo "Host: $(HOSTNAME) ($(OS)/$(ARCH))"
	@echo ""
	@echo "Available targets:"
	@echo "  make all               - Build all components (frontend, ttyd, cc-connect, cc-switch, backend)"
	@echo "  make frontend          - Build frontend assets (html/) and generate modules/ttyd/src/html.h"
	@echo "  make ttyd              - Compile native ttyd C server natively on the current host"
	@echo "  make cc-connect        - Compile cc-connect Go daemon (with web assets)"
	@echo "  make cc-connect-noweb  - Compile cc-connect Go daemon (WITHOUT rebuilding web assets)"
	@echo "  make cc-switch         - Compile cc-switch Rust CLI sidecar"
	@echo "  make backend           - Compile 1agents Go server (backend) with version ldflags"
	@echo "  make package           - Create a target-distinguished deployable archive in dist/"
	@echo "  make clean             - Clean all intermediate and build outputs across components"

all: frontend ttyd cc-connect cc-switch backend

frontend:
	@echo "=== Building Frontend (html/)..."
	cd html && corepack enable && yarn install && yarn build

ttyd:
	@echo "=== Building ttyd terminal server..."
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		cmake -DCMAKE_PREFIX_PATH="/opt/homebrew;/usr/local" -DCMAKE_BUILD_TYPE=Release -B build-ttyd -S modules/ttyd ; \
	fi
	make -C build-ttyd
	@mkdir -p build
	cp build-ttyd/ttyd build/ttyd
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/ttyd ===" ; \
		codesign --force --deep --sign - build/ttyd ; \
	fi

cc-connect:
	@echo "=== Building cc-connect daemon..."
	$(MAKE) -C modules/cc-connect build
	@mkdir -p build
	cp modules/cc-connect/cc-connect build/cc-connect
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/cc-connect ===" ; \
		codesign --force --deep --sign - build/cc-connect ; \
	fi

cc-connect-noweb:
	@echo "=== Building cc-connect daemon (no web build)..."
	$(MAKE) -C modules/cc-connect build-noweb
	@mkdir -p build
	cp modules/cc-connect/cc-connect build/cc-connect
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/cc-connect ===" ; \
		codesign --force --deep --sign - build/cc-connect ; \
	fi

cc-switch:
	@echo "=== Building cc-switch CLI..."
	cargo build --manifest-path modules/cc-switch-cli/src-tauri/Cargo.toml --release
	@mkdir -p build
	cp modules/cc-switch-cli/src-tauri/target/release/cc-switch build/cc-switch
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/cc-switch ===" ; \
		codesign --force --deep --sign - build/cc-switch ; \
	fi

backend:
	@echo "=== Building 1agents Go server (backend)..."
	mkdir -p build
	cd backend && go build -ldflags "$(AGENT_LDFLAGS)" -o ../build/1agents ./cmd/backend
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/1agents ===" ; \
		codesign --force --deep --sign - build/1agents ; \
	fi

agent: backend

package: all
	@echo "=== Packaging 1agents for $(OS_LOWER)-$(ARCH_LOWER) on $(HOSTNAME)..."
	@rm -rf dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	@mkdir -p dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin
	cp build/1agents dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/ttyd dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/cc-connect dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/cc-switch dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp -r html/dist dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/dist
	cd dist && tar -czf 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	@echo "=== Created package: dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz"

clean:
	@echo "=== Cleaning build artifacts..."
	rm -rf build build-ttyd dist
	rm -rf html/dist modules/ttyd/src/html.h
	$(MAKE) -C modules/cc-connect clean
	rm -rf src-tauri/resources src-tauri/target
	cargo clean --manifest-path modules/cc-switch-cli/src-tauri/Cargo.toml

.PHONY: tauri-resources tauri-dev tauri-build

tauri-resources: all
	@echo "=== Rebuilding frontend for Tauri (Desktop Mode) ==="
	cd html && corepack enable && yarn install && IS_DESKTOP=true yarn build
	@echo "=== Setting up Tauri resources ==="
	./scripts/setup-resources.sh

tauri-dev: tauri-resources
	@echo "=== Starting Tauri in development mode ==="
	npx @tauri-apps/cli dev

tauri-dev-dual: tauri-resources
	@echo "=== Starting 1agents Go daemon in background ==="
	./build/1agents -ttyd-bin ./build/ttyd -static html/dist -listen 0.0.0.0:38080 & \
	DAEMON_PID=$$! ; \
	trap "echo 'Stopping Go daemon...'; kill $$DAEMON_PID 2>/dev/null" EXIT INT TERM ; \
	echo "Waiting for Go daemon to bind..." ; \
	sleep 1.5 ; \
	echo "Starting Tauri desktop app..." ; \
	npx @tauri-apps/cli dev

tauri-build: tauri-resources
	@echo "=== Building Tauri production bundle ==="
	npx @tauri-apps/cli build
