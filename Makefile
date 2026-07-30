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

# Source file declarations for smart incremental builds
BACKEND_SRCS  := $(shell find backend -type f 2>/dev/null)
FRONTEND_SRCS := $(shell find frontend/src frontend/packages modules/1acp -type f 2>/dev/null)
TTYD_SRCS     := $(shell find modules/ttyd/src -type f 2>/dev/null)
CONNECT_SRCS  := $(shell find modules/cc-connect -type f ! -name "test_*" ! -path "*/.*" 2>/dev/null)
SWITCH_SRCS   := $(shell find modules/cc-switch-cli/src-tauri/src -type f 2>/dev/null)
HARNESSKIT_SRCS := $(shell find modules/HarnessKit/crates -type f 2>/dev/null) modules/HarnessKit/Cargo.toml modules/HarnessKit/Cargo.lock
COFFEE_SRCS   := $(shell find modules/alipay-coffee/src modules/alipay-coffee/public -type f 2>/dev/null) modules/alipay-coffee/package.json modules/alipay-coffee/package-lock.json

.PHONY: all frontend ttyd cc-connect cc-connect-noweb cc-switch harnesskit harnesskit-compliance harnesskit-sbom happy coffee backend agent package release-notes clean help install-hooks submodules submodule-cc-connect submodule-cc-switch submodule-happy-cli submodule-harnesskit

help:
	@echo "Unified Build and Packaging System for Remote Agents"
	@echo "Host: $(HOSTNAME) ($(OS)/$(ARCH))"
	@echo ""
	@echo "Available targets:"
	@echo "  make all               - Build all components (frontend, ttyd, cc-connect, cc-switch, HarnessKit, backend)"
	@echo "  make submodules        - Init/update all git submodules"
	@echo "  make frontend          - Build frontend assets (frontend/) and generate modules/ttyd/src/html.h"
	@echo "  make ttyd              - Compile native ttyd C server natively on the current host"
	@echo "  make cc-connect        - Compile cc-connect Go daemon (with web assets)"
	@echo "  make cc-connect-noweb  - Compile cc-connect Go daemon (WITHOUT rebuilding web assets)"
	@echo "  make cc-switch         - Compile cc-switch Rust CLI sidecar"
	@echo "  make harnesskit        - Compile the controlled HarnessKit hk CLI/web daemon"
	@echo "  make harnesskit-compliance - Verify protected HarnessKit artwork is absent"
	@echo "  make harnesskit-sbom   - Generate the locked HarnessKit SPDX dependency SBOM"
	@echo "  make happy             - Build the happy-cli Node submodule + build/happy launcher"
	@echo "  make coffee            - Stage the Alipay coffee payment Node.js service"
	@echo "  make backend           - Compile 1agents Go server (backend) with version ldflags"
	@echo "  make package           - Create a target-distinguished deployable archive in dist/"
	@echo "  make release-notes     - Generate a self-contained release feature-intro HTML page (FROM/TO/OUT overridable)"
	@echo "  make clean             - Clean all intermediate and build outputs across components"
	@echo "  make install-hooks     - Install git hooks (auto-push submodules + create PRs on git push)"

all: frontend ttyd cc-connect cc-switch harnesskit happy coffee backend

# --- Git submodules ---------------------------------------------------------
submodules:
	@echo "=== Initializing/updating all git submodules..."
	git submodule update --init --recursive

submodule-cc-connect:
	@echo "=== Ensuring cc-connect submodule is checked out..."
	git submodule update --init modules/cc-connect

submodule-cc-switch:
	@echo "=== Ensuring cc-switch-cli submodule is checked out..."
	git submodule update --init modules/cc-switch-cli

submodule-happy-cli:
	@echo "=== Ensuring happy-cli submodule is checked out..."
	git submodule update --init modules/happy-cli

submodule-harnesskit:
	@echo "=== Ensuring controlled HarnessKit submodule is checked out..."
	git submodule update --init modules/HarnessKit

frontend: frontend/dist/index.html

frontend/dist/index.html: $(FRONTEND_SRCS)
	@echo "=== Building Frontend (frontend/)..."
	cd frontend && corepack enable && yarn install && yarn build
	@echo "=== Staging module embeds (HarnessKit + cc-connect) into frontend/dist/embed..."
	./scripts/build-module-embeds.sh

ttyd: build/ttyd

build/ttyd: $(TTYD_SRCS)
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

cc-connect: build/cc-connect

build/cc-connect: $(CONNECT_SRCS)
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

cc-switch: build/cc-switch

build/cc-switch: $(SWITCH_SRCS)
	@echo "=== Building cc-switch CLI..."
	cargo build --manifest-path modules/cc-switch-cli/src-tauri/Cargo.toml --release
	@mkdir -p build
	cp modules/cc-switch-cli/src-tauri/target/release/cc-switch build/cc-switch
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/cc-switch ===" ; \
		codesign --force --deep --sign - build/cc-switch ; \
	fi

harnesskit: build/hk

build/hk: $(HARNESSKIT_SRCS)
	@echo "=== Building HarnessKit hk daemon/CLI..."
	cargo build --manifest-path modules/HarnessKit/Cargo.toml --release -p hk-cli
	@mkdir -p build
	cp modules/HarnessKit/target/release/hk build/hk
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/hk ===" ; \
		codesign --force --deep --sign - build/hk ; \
	fi

harnesskit-compliance:
	./scripts/check-harnesskit-artifacts.sh $(if $(wildcard frontend/dist),frontend/dist)

harnesskit-sbom:
	node ./scripts/generate-harnesskit-sbom.mjs build/compliance/harnesskit.spdx.json

happy: build/happy

build/happy:
	@echo "=== Building happy bundle (modules/happy-cli -> build/happy-cli + build/adapter)..."
	./scripts/build-happy-bundle.sh

coffee: build/alipay-coffee/package.json

build/alipay-coffee/package.json: $(COFFEE_SRCS)
	@echo "=== Building Alipay coffee payment bundle..."
	./scripts/build-alipay-coffee-bundle.sh

backend: build/1agents

build/1agents: $(BACKEND_SRCS)
	@echo "=== Building 1agents Go server (backend)..."
	mkdir -p build
	cd backend && go build -ldflags "$(AGENT_LDFLAGS)" -o ../build/1agents ./cmd/backend
	@if [ "$(OS_LOWER)" = "darwin" ]; then \
		echo "=== Ad-hoc signing build/1agents ===" ; \
		codesign --force --deep --sign - build/1agents ; \
	fi

agent: backend

package: all harnesskit-sbom
	@echo "=== Packaging 1agents for $(OS_LOWER)-$(ARCH_LOWER) on $(HOSTNAME)..."
	@rm -rf dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	@mkdir -p dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin \
		dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/config
	cp build/1agents dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/ttyd dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/cc-connect dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/cc-switch dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/hk dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp build/happy dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/
	cp -r build/happy-cli dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/happy-cli
	cp -r build/adapter dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/adapter
	cp -r build/alipay-coffee dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/bin/alipay-coffee
	cp -r frontend/dist dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/dist
	cp config/agent-extension-map.json dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/config/
	./scripts/stage-harnesskit-compliance.sh dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)/licenses
	cd dist && tar -czf 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz 1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME)
	./scripts/check-harnesskit-artifacts.sh dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz
	@echo "=== Created package: dist/1agents-$(VERSION)-$(OS_LOWER)-$(ARCH_LOWER)-$(HOSTNAME).tar.gz"

# --- Release feature-intro HTML page (issue #145) ---------------------------
# Generates a self-contained HTML page introducing the features in a release,
# from squash-merge commit subjects (feat(x): ... (#NNN)). Override the range
# and output with FROM / TO / OUT, e.g.:
#   make release-notes FROM=v20260623-1 TO=HEAD OUT=dist/release.html
# FROM defaults to the previous tag; OUT defaults to dist/release-notes.html.
FROM ?=
TO   ?= HEAD
OUT  ?= dist/release-notes.html
release-notes:
	@echo "=== Generating release feature-intro page -> $(OUT)..."
	@mkdir -p $(dir $(OUT))
	cd backend && go run ./cmd/release-notes $(if $(FROM),-from $(FROM),) -to $(TO) -o $(abspath $(OUT))

install-hooks:
	@echo "=== Installing git hooks..."
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-push
	@echo "Hooks installed. 'git push' will now auto-check submodules."

clean:
	@echo "=== Cleaning build artifacts..."
	rm -rf build build-ttyd dist
	rm -rf frontend/dist modules/ttyd/src/html.h
	$(MAKE) -C modules/cc-connect clean
	rm -rf src-tauri/resources src-tauri/target
	cargo clean --manifest-path modules/cc-switch-cli/src-tauri/Cargo.toml
	cargo clean --manifest-path modules/HarnessKit/Cargo.toml

.PHONY: tauri-resources tauri-dev tauri-build

tauri-resources: all
	@echo "=== Rebuilding frontend for Tauri (Desktop Mode) ==="
	cd frontend && corepack enable && yarn install && IS_DESKTOP=true yarn build
	@echo "=== Setting up Tauri resources ==="
	./scripts/setup-resources.sh

tauri-dev: tauri-resources
	@echo "=== Starting Tauri in development mode ==="
	npx @tauri-apps/cli dev

tauri-dev-dual: tauri-resources
	@echo "=== Starting 1agents Go daemon in background ==="
	./build/1agents -ttyd-bin ./build/ttyd -static frontend/dist -listen 0.0.0.0:38080 & \
	DAEMON_PID=$$! ; \
	trap "echo 'Stopping Go daemon...'; kill $$DAEMON_PID 2>/dev/null" EXIT INT TERM ; \
	echo "Waiting for Go daemon to bind..." ; \
	sleep 1.5 ; \
	echo "Starting Tauri desktop app..." ; \
	npx @tauri-apps/cli dev

tauri-build: tauri-resources
	@echo "=== Building Tauri production bundle ==="
	npx @tauri-apps/cli build
