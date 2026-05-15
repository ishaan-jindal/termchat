# termchat

Anonymous ephemeral terminal chatrooms.

Open terminal → paste one command → instantly chat.

---

# Features

* Anonymous realtime chat rooms
* Terminal-native TUI
* Cross-platform bootstrap system
* Linux / macOS / Windows / Android (Termux) support
* Ephemeral rooms
* Nickname colors
* Slash commands
* Dockerized deployment
* GitHub Actions CI/CD
* GHCR container deployment

---

# Demo

## Linux / macOS

Create room:

```bash
curl https://localhost | bash
```

Join room:

```bash
curl https://localhost/FROG | bash
```

---

## Windows (PowerShell)

Create room:

```powershell
irm https://localhost/win -OutFile termchat-bootstrap.ps1
.\termchat-bootstrap.ps1
```

Join room:

```powershell
irm https://localhost/win/FROG -OutFile termchat-bootstrap.ps1
.\termchat-bootstrap.ps1
```

If PowerShell blocks scripts:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
```

---

## Android / Termux

```bash
curl https://localhost | bash
```

---

# Supported Platforms

| Platform         | Architecture |
| ---------------- | ------------ |
| Linux            | amd64        |
| Linux            | arm64        |
| Linux            | 386 / i686   |
| macOS            | amd64        |
| macOS            | arm64        |
| Windows          | amd64        |
| Windows          | arm64        |
| Android / Termux | arm64        |

---

# Screenshots

```text
┌ Room: FROG ──────────────────────┐
│ alice: hello                    │
│ bob: hi                         │
│                                  │
├──────────────────────────────────┤
│ >                                │
└──────────────────────────────────┘
```

---

# Slash Commands

| Command       | Description           |
| ------------- | --------------------- |
| `/help`       | Show help             |
| `/clear`      | Clear chat            |
| `/users`      | Show online users     |
| `/nick NAME`  | Change nickname       |
| `/color #HEX` | Change nickname color |
| `/quit`       | Exit                  |

---

# Architecture

```text
GitHub Actions
    ↓
GHCR + GitHub Releases
    ↓
API server
    ↓
Bootstrap scripts
    ↓
CLI binaries
    ↓
WebSocket server
```

---

# Project Structure

```text
termchat/
├── api/
├── cli/
├── server/
├── scripts/
│   ├── bootstrap.sh
│   └── bootstrap.ps1
├── caddy/
├── .github/workflows/
├── Dockerfile.api
├── Dockerfile.server
├── docker-compose.yml
├── .env.example
└── README.md
```

---

# Local Development

## Requirements

* Go 1.26+
* Docker
* Docker Compose

---

# Run WebSocket Server

```bash
cd server
go run .
```

---

# Run API Server

```bash
cd api
go run .
```

---

# Run CLI

```bash
cd cli
go run .
```

---

# Environment Variables

Example `.env`:

```env
WS_HOST=0.0.0.0
WS_PORT=8080

API_PORT=3000

PUBLIC_API_URL=https://localhost
PUBLIC_WS_URL=wss://localhost/ws

GITHUB_REPO=YOUR_USERNAME/termchat
RELEASE_VERSION=v0.1.0
```

---

# Docker Deployment

## Start Stack

```bash
docker compose up -d
```

---

## Pull Updated Images

```bash
docker compose pull
docker compose up -d
```

---

# Caddy

Example `Caddyfile`:

```text
localhost {

    reverse_proxy /ws* websocket:8080

    reverse_proxy api:3000
}
```

---

# CI/CD

GitHub Actions automatically:

* Builds CLI binaries
* Publishes GitHub Release assets
* Builds Docker images
* Pushes images to GHCR

Triggered via tags:

```bash
git tag v0.1.0
git push origin v0.1.0
```

---

# Security Notes

Current implementation includes:

* WebSocket keepalive
* Buffered message queues
* Cross-platform bootstrap detection

Still recommended before public exposure:

* ANSI escape sanitization
* Message length limits
* Rate limiting
* Room validation hardening

---

# Technologies

* Go
* Gorilla WebSocket
* Bubble Tea
* Lip Gloss
* Chi
* Docker
* Caddy
* GitHub Actions
* GHCR

---

# License

MIT

