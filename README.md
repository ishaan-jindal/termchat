# termchat

[![CI](https://github.com/ishaan-jindal/termchat/actions/workflows/ci.yml/badge.svg)](https://github.com/ishaan-jindal/termchat/actions?query=event%3Apush+branch%3Amain)

Minimal anonymous terminal chatrooms. Open a terminal, paste one command, and
instantly chat with anyone who has the room code.

Built for quick collaboration, debugging sessions, pair programming, and
temporary communities - no signup, no browser tabs.

## Why termchat?

- Anonymous ephemeral rooms with zero account creation
- One command to install and join: `curl -fsSL <host> | bash`
- A single server binary serves everything: WebSocket rooms, room
  discovery, and the bootstrap installer flow
- LAN host mode runs a server in your terminal, no deployment needed
- Linux, macOS, Windows, and Android (Termux) support

## Features

Status: **Active**. Server and client, nothing else - the API server was
removed; rooms, discovery, and bootstrap all live in one binary.

**Chat:**

- Realtime WebSocket messaging with in-memory history (last 30 messages)
- Room passwords (locked / unlocked rooms) with interactive prompt on join
- Host privileges with automatic succession on host disconnect
- Room discovery: online via `/discover`, LAN via UDP multicast beacon

**Voice:**

- Push-to-talk voice chat in the same window: `/voice on`, then `Ctrl+T`
  toggles transmit; muting kills the capture process so the OS shows the
  mic released
- 16 kHz mono PCM in 40 ms chunks over a binary `/media` WebSocket,
  authenticated with single-use tokens from the control socket; playback
  prefers `paplay` on Linux and falls back to `ffplay` elsewhere
- Overlapping speakers are mixed locally per peer, with `[mic]` markers in
  the sidebar and a `VOICE [TX]` badge in the status footer
- Requires `ffmpeg` (capture) and `ffplay` (playback) on the PATH;
  platforms without them simply keep chat-only mode

**Terminal UI:**

- Modern Bubble Tea TUI with a users sidebar
- Mention highlighting (`@nickname`), nickname colors, input history
- Typing indicators and a status footer
- Color themes: system (terminal-native, adaptive accents) or forced
  palettes like dracula, nord, gruvbox, dark, light

**Hosting:**

- Cloud rooms against any server: `termchat`
- LAN host mode: `termchat host` embeds the server in-process
- Self-hostable with Docker Compose: websocket + caddy + watchtower, with
  automatic HTTPS
- The `curl | bash` one-liner is served by your own server and points
  clients at it automatically (`PUBLIC_BASE_URL`)

**Delivery:**

- GitHub Releases binary delivery for all platforms with a checksum file
- AUR package (`ishaans-termchat-bin`)
- GHCR container image

## Quick Start

Create a room:

```bash
curl -fsSL https://termchat.sacred99.online | bash
```

Join a room:

```bash
curl -fsSL https://termchat.sacred99.online/7WHB | bash
```

Windows (PowerShell):

```powershell
irm https://termchat.sacred99.online/win -OutFile termchat-bootstrap.ps1
.\termchat-bootstrap.ps1
```

Or install via npm (`npm i -g @sacredcat/termchat`), grab a binary directly
from the [Releases Page](https://github.com/ishaan-jindal/termchat/releases),
or on Arch Linux use the AUR (`ishaans-termchat-bin`).

Then:

```bash
termchat          # create a new room
termchat FROG     # join a room
termchat host     # host a LAN room
termchat discover # list online and LAN rooms
```

## Quick Links

- [CLI Usage](#cli-usage) - commands and flags
- [LAN Host Mode](#lan-host-mode) - host rooms from your terminal
- [Docker](#docker) - self-host the full stack
- [Man page](doc/termchat.1) - installed with `make install`
- [Changelog](CHANGELOG.md) - what changed

## CLI Usage

```bash
termchat                  # create a new room on the default server
termchat FROG             # join a room
termchat --room FROG      # join with an explicit flag
termchat FROG --server wss://my.server/ws   # custom server
termchat host [ROOM]      # LAN host mode (embeds the server)
termchat host --password secret             # lock the room
termchat FROG --host 192.168.1.42           # join a LAN host
termchat discover         # list online and LAN rooms
termchat discover --online | --local        # filter
```

In-room commands: `/help`, `/clear`, `/nick NAME`, `/color #HEX`,
`/theme [NAME]`, `/password [NEWPASS]` (host only), `/users` (list who is in
the room), `/reply ID MESSAGE` (quote a message), `/react ID REACTION`
(react to a message), `/voice on|off` (voice session; `Ctrl+T` toggles
transmit), `/quit`.

Each chat message is tagged with its ID (e.g. `#7 bob: hello world`), so
`/reply 7 ...` quotes it and `/react 7 +1` reacts to it. Reactions are
per-user toggles; supported names: `+1`, `-1`, `laugh`, `heart`, `wow`,
`eyes`, `fire`, `clap`.

## Themes

The default `system` theme keeps your terminal's own colors and adapts only
the accents to light or dark backgrounds. Built-in named themes - `dark`,
`light`, `dracula`, `nord`, `gruvbox` and more - force their own palette
over the entire window, so the terminal's own colors do not show through.

```bash
termchat --theme gruvbox   # pick a theme for this session
```

Or switch mid-chat with `/theme dracula`; `/theme` alone lists every theme
with a color swatch preview. The choice is saved to
`~/.termchat/config.json`.

## LAN Host Mode

LAN Host Mode runs the WebSocket server, room manager, and local TUI in one
process. Other users connect directly to your IP:

```bash
termchat host FROG --port 9000 --password secret
```

A UDP beacon is announced on every network interface each second, over both
multicast and broadcast, so `termchat discover --local` finds your room.
`--server` takes priority over `--host` / `--port`.

LAN discovery only crosses one link: it cannot see through routers, NAT
hotspots (e.g. a travel router sharing a dorm wifi uplink), or AP isolation.
Host and joiners must be on the same subnet; otherwise connect directly:

```bash
termchat FROG --host 192.168.1.42 --port 9000
```

## Room System

Rooms are temporary and memory-only: created on join, deleted when empty,
capped at 30 history messages. The first joiner becomes host; when the host
disconnects, the next-oldest client succeeds (broadcast as a system
message). Rooms are shareable via URL-style codes:

```text
https://termchat.sacred99.online/7WHB
```

## Security

- ANSI escape and control-character sanitization, rune-safe truncation
- Message length enforcement (500 runes) and a 4KB frame read limit
- Room passwords for access control, 5 msgs/sec per-client rate limit
- Idle connection cleanup (30 min) and buffered, shutdown-aware sends
- Binary-name whitelist on `/bin/{binary}` redirects
- Voice sessions require single-use tokens bound to the chat connection,
  with per-connection frame-size and bandwidth caps on `/media`

Recommended future hardening: global + per-room rate limits, join
throttling, profanity / spam filtering, abuse detection.

## Building from Source

Use `just` (recommended) or the Makefile:

```bash
just check     # full CI gate: tidy, fmt, vet, build, race-tested tests
just build     # CLI binary to dist/termchat
just cross     # cross-compile all 8 release platforms
just server    # run the WebSocket server locally (port 8080)
```

```bash
make build     # Build CLI binary to dist/termchat
make install   # Install CLI, man page, and license
```

## Docker

Self-host the full stack with Docker Compose:

```bash
cp .env.example .env
# Edit .env with your settings
docker-compose up -d
```

Services: `websocket` (rooms, discovery, bootstrap scripts), `caddy`
(reverse proxy with automatic HTTPS), `watchtower` (automatic updates).

Configuration (see `.env.example`):

| Variable          | Purpose                                            |
| ----------------- | -------------------------------------------------- |
| `WS_HOST`         | Listen address (default `0.0.0.0`)                 |
| `WS_PORT`         | Listen port (default `8080`)                       |
| `PUBLIC_BASE_URL` | Public origin; the served one-liner downloads binaries and points clients at `PUBLIC_BASE_URL/ws` |
| `GITHUB_REPO`     | Repo for release binary downloads (default `ishaan-jindal/termchat`) |

Images are published to [GHCR](https://github.com/users/ishaan-jindal/packages/container/package/termchat-websocket).

## Supported Platforms

| Platform         | Architectures            |
| ---------------- | ------------------------ |
| Linux            | amd64, arm64, 386 / i686 |
| macOS            | amd64, arm64             |
| Windows          | amd64, arm64             |
| Android / Termux | arm64                    |

## Contributing

Fixes and new features are greatly appreciated. Make sure to read our
[contributing guidelines](CONTRIBUTING.md) first - commits must be signed
(`git commit -s -S`) and pass `just pre-commit`.

## Credits

This project uses AI tools as development aids (drafting, iteration,
reviews, tests, and documentation). Architecture, constraints, and final
code decisions are owned by the human committers.

The mobile companion is [termchat-mobile](https://github.com/ishaan-jindal/termchat-mobile).

## License

[MIT](LICENSE)
