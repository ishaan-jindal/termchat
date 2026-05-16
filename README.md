# termchat

Minimal anonymous terminal chatrooms.

Open a terminal → paste one command → instantly chat.

Built for quick collaboration, debugging sessions, pair programming, temporary communities, and internet-native realtime chat.

---

# Features

* Anonymous ephemeral chat rooms
* Zero account creation
* Realtime WebSocket messaging
* Modern terminal-native TUI
* Sidebar user list
* Mention highlighting (`@user`)
* Cross-platform bootstrap installer
* Linux / macOS / Windows / Android (Termux) support
* Auto-generated room IDs
* Nickname colors
* Responsive terminal layout
* GitHub Releases binary delivery
* Dockerized deployment
* GitHub Actions CI/CD
* GHCR container publishing
* Lightweight single-binary CLI

---

# UI Preview

The latest TUI includes:

* Dedicated users sidebar
* Better spacing and layout
* Cleaner message rendering
* Improved command hints
* Status footer
* Mention highlighting
* Better input handling
* Adaptive resizing

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ [system] Alice joined the room                                             │
│ Alice: Hey @Bob                                                            │
│ Bob: sup                                                                   │
│                                                                            │
│ Commands: /help /clear /nick /color /quit                                  │
├────────────────────────────────────────────────────────────────────────────┤
│ > Type a message...                                                        │
└────────────────────────────────────────────────────────────────────────────┘
```

---

# Quick Start

## Linux / macOS

Create a room:

```bash
curl -fsSL https://termchat.sacred99.online | bash
```

Join a room:

```bash
curl -fsSL https://termchat.sacred99.online/7WHB | bash
```

---

## Windows (PowerShell)

Create a room:

```powershell
irm https://termchat.sacred99.online/win -OutFile termchat-bootstrap.ps1
.\termchat-bootstrap.ps1
```

Join a room:

```powershell
irm https://termchat.sacred99.online/win/7WHB -OutFile termchat-bootstrap.ps1
.\termchat-bootstrap.ps1
```

If PowerShell blocks scripts:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
```

---

## Android / Termux

```bash
pkg install curl
curl -fsSL https://termchat.sacred99.online | bash
```

---

# Supported Platforms

| Platform         | Architectures            |
| ---------------- | ------------------------ |
| Linux            | amd64, arm64, 386 / i686 |
| macOS            | amd64, arm64             |
| Windows          | amd64, arm64             |
| Android / Termux | arm64                    |

---

# Commands

| Command       | Description             |
| ------------- | ----------------------- |
| `/help`       | Show available commands |
| `/clear`      | Clear chat history      |
| `/nick NAME`  | Change nickname         |
| `/color #HEX` | Change nickname color   |
| `/quit`       | Exit room               |

Notes:

* The online users panel is built directly into the UI.
* `/users` command has been removed.
* Mentions highlight automatically when using `@nickname`.

---

# Room System

termchat rooms are:

* Temporary
* Memory-only
* Automatically created on join
* Deleted when empty
* Shareable via URL-style room codes

Example:

```text
https://termchat.sacred99.online/7WHB
```

---

# Security

Current protections include:

* WebSocket keepalive
* Buffered outbound queues
* Graceful disconnect handling
* Inactive connection cleanup
* Cross-platform bootstrap detection
* Automatic binary fetching
* ANSI escape sanitization
* Message length enforcement

Recommended future hardening:

* Global + per-room rate limits
* Join throttling
* Room validation hardening
* Profanity / spam filtering
* Abuse detection

---

# Roadmap

Planned ideas:

* File transfer
* Message reactions
* Terminal notifications
* Persistent optional identities
* Invite-only rooms
* End-to-end encryption experiments
* Self-hosted one-command deployment
* Rich markdown rendering
* Multi-room support

---

# Technologies

* Go
* Bubble Tea
* Lip Gloss
* Gorilla WebSocket
* Chi
* Docker
* Caddy
* GitHub Actions
* GitHub Container Registry

---

# Philosophy

termchat is designed to feel:

* Instant
* Disposable
* Lightweight
* Terminal-first
* Frictionless

No signup.
No browser tabs.

---

# License

MIT
