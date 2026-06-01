---
name: 1agents-tunnel
description: Expose local services to the public internet via Cloudflare Tunnel (cloudflared). Use when an AI agent or external service needs to access a local HTTP service running on the 1agents host from the public internet. Supports starting tunnels on specific ports, stopping them, checking status, and multi-port concurrent tunnels with token-based access control.
---

# Remote Agents Tunnel

## Overview

Exposes local HTTP services running on the 1agents host to the public internet using Cloudflare Quick Tunnels. Each tunnel creates a unique `trycloudflare.com` URL with a session token for access control.

## Quick Start

```bash
# Start a tunnel on the default port (8087)
./agent/1agents tunnel start

# Start a tunnel on a specific port
./agent/1agents tunnel start 7681

# Start with idle timeout (minutes, -1=never, 0=default)
./agent/1agents tunnel start 8087 30

# Check tunnel status
./agent/1agents tunnel status

# Stop a tunnel
./agent/1agents tunnel stop 8087

# Stop all tunnels
./agent/1agents tunnel stop-all
```

## Core Capabilities

### 1. Start Tunnel

**CLI:** `tunnel start [port] [timeout]`

- `port`: Local port to expose. Defaults to daemon port (8087).
- `timeout`: Idle timeout in minutes. `0`=global default, `-1`=never expire.

**API:** `POST /api/tunnel/start?port=8087&timeout=15`

**Response:**
```json
{
  "port": "8087",
  "url": "https://plants-spiritual-transmit-simulation.trycloudflare.com",
  "token": "51f819b4f6bc6fba6b12f6963889542a",
  "link": "https://plants-spiritual-transmit-simulation.trycloudflare.com/?token=51f819b4f6bc6fba6b12f6963889542a"
}
```

The `link` field is the full public URL. The `token` is required for access when tunnel auth is active.

### 2. Stop Tunnel

**CLI:** `tunnel stop <port>`

**API:** `POST /api/tunnel/stop?port=8087`

### 3. Stop All Tunnels

**CLI:** `tunnel stop-all`

**API:** `POST /api/tunnel/stop-all`

### 4. Tunnel Status

**CLI:** `tunnel status`

**API:** `GET /api/tunnel/status`

**Response:**
```json
{
  "active": true,
  "tunnels": [
    {
      "port": "8087",
      "url": "https://tomato-nasa-pen-reflected.trycloudflare.com",
      "token": "18ad2ee9c1826272d928e6094043617d",
      "link": "https://tomato-nasa-pen-reflected.trycloudflare.com/?token=18ad2ee9c1826272d928e6094043617d",
      "idle_seconds": 898
    }
  ]
}
```

`idle_seconds`: seconds until auto-stop. `0` means never expires. `-1` means no idle tracking.

## Access Control

When any tunnel is active, the daemon enforces token-based access:

- **URL parameter**: `?token=<session_token>`
- **Authorization header**: `Bearer <session_token>`
- **Cookie**: `ra_session_token`

The session token from `/api/tunnel/start` response grants access. The token expires when the tunnel stops.

## Multi-Port Tunnels

Multiple tunnels can run concurrently, each on a different local port:

```bash
./agent/1agents tunnel start 8087   # Tunnel 1
./agent/1agents tunnel start 7681   # Tunnel 2 (separate URL)
```

Each tunnel is independent with its own URL and token.

## Architecture

```
Local Service (port 8087)
       ↓
  cloudflared process
       ↓
Cloudflare Edge (trycloudflare.com)
       ↓
Public User → https://xxx.trycloudflare.com/?token=<session>
```

- **Supervisor**: `agent/internal/tunnel/supervisor.go` — manages tunnel lifecycle
- **CLI**: `agent/cmd/agent/tunnel_cmd.go` — CLI entry point
- **API handlers**: `agent/internal/server/server.go` — HTTP API
- **Auth**: Uses `ccconnect.ManagementToken` for API authorization

## Limitations

- **Quick Tunnel** (current mode): No account required. Limits are per-IP rate limits on tunnel creation. Creating multiple tunnels in quick succession may return HTTP 500 "timeout waiting for tunnel to establish". Retry after a short delay.
- **Named Tunnel** (Zero Trust): If higher limits needed, switch to Cloudflare Zero Trust named tunnels (max 3 free).
- Each tunnel URL can handle thousands of concurrent connections — bottleneck is local bandwidth.
