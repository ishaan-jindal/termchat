# termchat

Anonymous ephemeral terminal chatrooms.

Open website → copy command → paste into terminal → instant shared chat room.

No accounts.
No installs.
No persistence.
No friction.

---

# Concept

`termchat` is a lightweight real-time terminal chat system built around temporary rooms.

Users create or join rooms through a website, receive a one-line bootstrap command, and instantly connect through a terminal UI client.

The product is intentionally:

* anonymous
* disposable
* fast
* terminal-native

Think:

* IRC without setup
* Discord stripped to essentials
* multiplayer terminal session vibes

---

# Core User Flow

## Create Room

User visits website.

Clicks:

```text
New Chat
```

Website returns:

```bash
curl -sSL https://termchat.app/r/FROG | bash
```

User runs it.

Client launches:

```text
Enter nickname:
> ghost
```

Connected:

```text
┌ Room: FROG ──────────────────────┐
│ alice: hey                      │
│ ghost: yo                       │
│                                  │
├──────────────────────────────────┤
│ >                                │
└──────────────────────────────────┘
```

User shares room code:

```text
FROG
```

---

## Join Room

User visits website.

Enters room code:

```text
FROG
```

Website returns:

```bash
curl -sSL https://termchat.app/j/FROG | bash
```

User launches terminal client and joins instantly.

---

# Product Philosophy

## Things This App SHOULD Be

* instant
* anonymous
* minimal
* temporary
* terminal-first
* hacker-ish
* fun
* low commitment

---

## Things This App SHOULD NOT Be

* social network
* account system
* persistent messaging platform
* Slack clone
* Discord clone
* enterprise software

The appeal is the lack of ceremony.

---

# Architecture

```text
Website
   ↓
Bootstrap Endpoint
   ↓
CLI/TUI Client
   ↓
WebSocket Server
   ↓
Room Broadcast System
```

---

# Components

# 1. Website

Tiny frontend.

Responsibilities:

* create room
* join room
* generate bootstrap commands
* explain product

No auth.
No dashboard.
No user management.

---

## Suggested Pages

### `/`

Landing page.

---

### `/new`

Creates room.

Returns:

* room code
* bootstrap command

---

### `/join`

Input field for room code.

Returns:

* join command

---

# 2. Bootstrap System

## Goal

Zero-install UX.

User pastes:

```bash
curl -sSL https://termchat.app/r/FROG | bash
```

Server returns shell script.

Script:

* detects platform
* downloads binary
* executes binary
* passes room code

---

## Example Bootstrap Script

```bash
#!/bin/bash

ARCH=$(uname -m)
OS=$(uname -s)

TMP=$(mktemp)

curl -sSL https://cdn.termchat.app/bin/linux-amd64 -o $TMP

chmod +x $TMP

$TMP --room FROG
```

---

# 3. CLI/TUI Client

## Responsibilities

* websocket connection
* rendering messages
* input handling
* reconnect logic
* slash commands
* nickname handling

---

## Suggested Stack

### Language

* Go

---

### TUI Framework

* Bubble Tea

---

### Styling

* Lip Gloss

---

# TUI Layout

```text
┌ Room: FROG ─────────────────────────┐
│                                     │
│ alice: hello                        │
│ ghost: hi                           │
│                                     │
│ bob joined the room                 │
│                                     │
├─────────────────────────────────────┤
│ >                                   │
└─────────────────────────────────────┘
```

---

# 4. WebSocket Server

Core realtime layer.

Handles:

* room creation
* room membership
* broadcasts
* disconnects
* cleanup

---

## Suggested Stack

### Go

Use:

* gorilla/websocket
  or
* nhooyr/websocket

---

# Room Structure

```go
type Room struct {
    ID        string
    Clients   map[*Client]bool
    CreatedAt time.Time
}
```

---

# Client Structure

```go
type Client struct {
    Conn     *websocket.Conn
    Nickname string
    RoomID   string
}
```

---

# Global Room Registry

```go
map[string]*Room
```

---

# Message Protocol

## Join Event

```json
{
  "type": "join",
  "nick": "ghost",
  "room": "FROG"
}
```

---

## Chat Message

```json
{
  "type": "message",
  "text": "hello"
}
```

---

## Broadcast Message

```json
{
  "type": "message",
  "nick": "ghost",
  "text": "hello",
  "timestamp": 1740000
}
```

---

## System Message

```json
{
  "type": "system",
  "text": "bob joined the room"
}
```

---

# Room Codes

Use:

* short
* memorable
* shareable

Good:

```text
FROG
LIME
K9X2
BYTE
NOVA
```

Bad:

```text
4f7c8d1b-e93e
```

---

# Room Lifecycle

Rooms are ephemeral.

---

## Suggested Cleanup Rules

Delete room if:

* no users remain
* inactive for 1 hour
* max lifespan exceeded

---

# Nicknames

Anonymous identity only.

Prompt on startup:

```text
Enter nickname:
>
```

---

## Rules

* no uniqueness required
* max length
* sanitize ANSI escape sequences
* sanitize control characters

---

# Slash Commands

## MVP Commands

```text
/nick newname
/users
/clear
/help
/quit
```

---

# Transport

## Use WebSockets

Do NOT:

* poll
* refresh
* use REST for messaging

Realtime terminal chat is exactly what WebSockets are for.

---

# State Management

Initial version can be fully in-memory.

No database required.

---

# Persistence

None.

Messages disappear when room dies.

That is part of the appeal.

---

# Infra

Very lightweight.

A single small VM can likely handle:

* hundreds/thousands of concurrent sockets

depending on implementation.

---

# Suggested Hosting

* [Fly.io](https://fly.io?utm_source=chatgpt.com)
* [Railway](https://railway.app?utm_source=chatgpt.com)
* [Hetzner](https://www.hetzner.com?utm_source=chatgpt.com)
* [DigitalOcean](https://www.digitalocean.com?utm_source=chatgpt.com)

---

# Binary Distribution

Build binaries for:

* linux-amd64
* linux-arm64
* macos-amd64
* macos-arm64

Optional later:

* windows

---

# Security Notes

## Important

Users are cautious about:

```bash
curl ... | bash
```

So:

* make bootstrap script visible on website
* open source the client
* keep bootstrap tiny
* avoid suspicious behavior

---

## Sanitize Input

Must sanitize:

* terminal escape sequences
* ANSI injection
* control characters

Otherwise users can:

* clear terminals
* move cursor
* spoof UI

---

# Suggested Features

# MVP

* anonymous rooms
* room codes
* realtime chat
* terminal UI
* join/leave events
* auto cleanup

---

# Nice Additions

## Colored Nicknames

Deterministic hash:

```text
alice -> blue
bob -> green
```

---

## Typing Indicator

```text
alice is typing...
```

---

## Online User List

```text
/users
```

---

## Reconnect Support

Recover after temporary disconnect.

---

## Sound Notifications

Terminal bell:

```text
\a
```

---

## Paste Support

Multiline pastes.

---

## Markdown-ish Formatting

Optional:

```text
*bold*
_italic_
`code`
```

---

# Features To Avoid

Avoid turning this into:

* Slack
* Discord
* Matrix
* IRC replacement

Overbuilding kills the gimmick.

---

# Branding Direction

## Themes

* hacker
* retro terminal
* cyberpunk
* minimal unix tooling

---

## Good Names

* termchat
* ttychat
* shelltalk
* ghostroom
* pipechat
* roomsh
* talksh
* cliq
* voidchat

---

# Future Ideas

# SSH Style Join

```bash
ssh termchat.app/FROG
```

---

# LAN Discovery

```bash
termchat --local
```

Using UDP broadcast.

---

# File Sharing

Upload through CLI.

Server returns:

```text
https://termchat.app/f/abc123
```

---

# Self Hosting

Docker image:

```bash
docker run termchat/server
```

---

# Philosophy Summary

The entire experience should feel like:

```text
open terminal
paste command
instantly talking
```

Everything else is secondary.

