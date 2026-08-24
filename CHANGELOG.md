# Changelog

## Unreleased

### Added

- `-v` as a shorthand for `--version` in the CLI. (by @ishaan-jindal)
- Server-stamped message IDs: every chat message carries an ID (e.g. `#7`),
  shown in the TUI and preserved in history replay. (by @ishaan-jindal)
- `/reply ID MESSAGE` quotes a message; the server resolves the quoted
  nick/text from history and the TUI renders it as a quote line. (by @ishaan-jindal)
- `/react ID REACTION` toggles per-user reactions, rendered inline as emoji
  like `[👍 x2]` and included in history replay. Supported names: +1, -1,
  laugh, heart, wow, eyes, fire, clap. (by @ishaan-jindal)
- Platform-specific system notifications (Linux, macOS, Windows) shown on
  new messages, plus TUI rendering for message display. (by @AaryanKumarSingh136)
- `/users` prints the current room roster into the chat log, with the host
  marked, so the participant list is visible without the sidebar. (by @AkulRanjan)
- Slash command and emoji autocomplete popup in the TUI: opens on `/`, `:`
  or Tab, filters as you type, Up/Down select, Tab/Enter accept and Esc
  dismisses. Commands live in a registry that also drives `/help`; shortcodes
  double as `/react` names and complete with their glyph. (by @ishaan-jindal)
- LAN discovery works on multi-homed hosts: beacons are sent on every
  eligible interface via multicast plus a broadcast fallback, listeners join
  the group on all interfaces, and an empty scan now explains the same-L2
  requirement and the direct-connect alternative.

### Fixed

- Join times stay correct when either machine's clock is skewed: users_list
  frames carry the server's time (`server_time`) and future timestamps
  render as "now" instead of negative durations.
- Nicknames are now strict: printable ASCII only, no spaces, up to 32
  characters. Invalid names are re-prompted at startup, rejected by `/nick`
  with feedback, and refused by the server (`invalid_nick`) instead of
  being silently accepted.

### Removed

## [cli-v2.0.0] - 2026-08-18

### Removed

- The separate API server is gone. The project is now server + client only:
  - `api/` and `Dockerfile.api` were deleted.
  - The CLI no longer calls `/api/new`; room codes are generated locally with
    `shared.GenerateRoomCode()`.
  - The `--api` flag and `main.DefaultAPI` ldflags were removed.
- The `curl | bash` one-liner install flow moved into the WebSocket server:
  - Bootstrap scripts live in `server/scripts/` (embedded via `go:embed`).
  - The server now serves `/`, `/{room}`, `/win`, `/win/{room}` and
    `/bin/{binary}` (with a binary-name whitelist).
  - Scripts render `{Room, BaseURL, Version}` from `PUBLIC_BASE_URL` and
    launch the binary with `--server`, so self-hosted deployments no longer
    point users at the default server.
  - The GitHub CLI version cache (5-min refresh) moved to the server, now
    mutex-guarded and with a client timeout.
- `docker-compose.yml` is now websocket + caddy + watchtower; the Caddyfile
  was fixed for the real domain (`termchat.sacred99.online`) with automatic
  HTTPS and a persistent ACME data volume.
- `justfile` no longer has an `api` recipe; `just docker` builds the server
  image only.

### Fixed

- Docker healthcheck used `/dev/tcp`, which dash (`/bin/sh` on Debian slim)
  does not support - containers were always reported unhealthy. The server
  image now installs `curl` and probes `/healthz`.
- Critical server crash: concurrent `rooms` map reads in the `set_password`
  and legacy `users` handlers could trigger a fatal "concurrent map read and
  map write" panic, disconnecting every room. All map access now happens
  under the registry lock.
- Server crash: "send on closed channel" panic when broadcasting to a
  disconnecting client. `client.Send` is never closed anymore; lifecycle is
  tracked with a per-client `done` channel (idempotent via `sync.Once`).
- Data races on client state (nickname, color, typing, last activity) -
  all mutable client fields are now guarded by a per-client mutex.
- Data race on the cached CLI version (the GitHub API version cache now
  lives in the server's bootstrap module) - guarded by `sync.RWMutex`.
- Client messages of any type were previously broadcast to the room. Only
  `message` is broadcast now; everything else is ignored.
- `GenerateRoomCode` had a modulo bias making characters A-D ~14% more
  likely. Replaced with rejection sampling for uniform distribution.
- `IsValidRoomCode` no longer silently normalizes input; it validates the
  exact code format (callers normalize first).
- UTF-8 runes are no longer split mid-sequence when truncating nicknames
  and messages.
- Bootstrap scripts are embedded into the server binary (`go:embed`) and
  no longer read from disk at request time.
- Health check endpoint (`/healthz`) on the WebSocket server.
- `scripts` moved under `server/scripts`; `go.work` removed - the project is
  a single Go module.
- Server lifecycle on shutdown: `Stop()` now blocks until the server has
  fully drained (listener down, connections closed) and the background
  loops (CLI version refresh, idle-client and typing cleanup) exit instead
  of leaking goroutines. Each `StartServer` gets a fresh room registry, so
  restarts never inherit stale rooms.

### Added

- The server now serves the bootstrap one-liner (`curl -fsSL <host> | bash`)
  and PowerShell flow directly; `/bin/{binary}` validates against a whitelist
  of the 8 published release assets.

By: @ishaan-jindal

## [cli-v1.1.1] - 2026-06-25

### Fixed

- Critical concurrency bugs: concurrent map access, WebSocket write races,
  and client field data races
- writePump panic: "close of closed channel" on /quit and Ctrl+C
- Remaining "use of closed network connection" errors on client disconnect
  (cleanupClient reordered to remove client before broadcasting)

### Added

- Graceful SIGTERM/SIGINT shutdown for both WebSocket server and API server

### CI

- API and WebSocket Docker builds now trigger on shared/ package changes

By: @ishaan-jindal

## [cli-v1.1.0] - 2026-06-04

### Added

- Room passwords (locked/unlocked rooms) with interactive prompt on join
- Host privileges with automatic succession on host disconnect
- LAN room discovery via `termchat discover --local`

### Changed

- Removed emoji rendering from TUI
- Updated documentation and man page for password and host features

By: @ishaan-jindal

## [cli-v1.0.1] - 2026-06-03

No user-facing changes.

By: @ishaan-jindal

## [cli-v1.0.0] - 2026-06-02

### Added

- LAN Host Mode: built-in WebSocket server, room manager, and TUI in one process
- `termchat host` command with auto-generated room codes
- LAN join via `--host` and `--port` flags
- UDP multicast beacon for local room discovery

By: @ishaan-jindal

## [cli-v0.4.4] - 2026-06-02

### Changed

- Extracted shared types into a `shared` module
- Fixed AUR publishing workflow

By: @ishaan-jindal

## [cli-v0.4.3] - 2026-06-01

### Added

- Typing indicator - shows `[...]` next to users currently typing

By: @ishaan-jindal

## [cli-v0.4.2] - 2026-05-31

### Added

- Makefile with build, install, uninstall targets
- Man page (`doc/termchat.1`)
- MIT License

### Changed

- Updated default API and WebSocket URLs

By: @ishaan-jindal

## [cli-v0.4.1] - 2026-05-19

### Fixed

- Mention highlighting now highlights the full message, not just the nickname

By: @ishaan-jindal

## [cli-v0.4.0] - 2026-05-19

### Added

- User list sidebar with colored nicknames, joined timestamps, and typing indicator
- User info broadcast (nickname, color, join time, typing status) from server

### Fixed

- `/color` now immediately broadcasts updated user list to all clients

By: @ishaan-jindal

## [cli-v0.3.0] - 2026-05-17

### Added

- Persistent config (`~/.termchat/config.json`) for nickname and color
- `--version` flag
- Multiline textarea input (Alt+Enter for newline)
- Standalone CLI UX improvements
- Updated API routes and bootstrap scripts for new features

By: @ishaan-jindal

## [cli-v0.2.1] - 2026-05-16

### Added

- Idle user cleanup (30 min timeout)
- Spam prevention (5 messages/second max)

### Fixed

- Mouse scroll support
- Non-command text starting with `/` now sent as normal message

### Changed

- API server periodically refreshes latest CLI version

By: @ishaan-jindal

## [cli-v0.2.0] - 2026-05-16

### Added

- Sidebar user list UI
- Status bar footer
- Mention highlighting (`@nickname`)
- Input history (Up/Down arrow)
- Server-side user list broadcast
- Responsive terminal layout improvements

### Fixed

- API server: added ca-certificates to runtime Docker image
- API server: fixed version detection

By: @ishaan-jindal

## [cli-v0.1.5] - 2026-05-15

### Added

- In-memory message history (last 30 messages sent to joining clients)
- ANSI escape sequence sanitization
- Control character filtering in input
- Full Docker deployment (server, API, Caddy)
- GitHub Actions CI/CD with GHCR container publishing
- Cross-platform binary releases via GitHub Releases
- Windows (PowerShell) bootstrap installer
- Android/Termux support
- Room code generation and sharing via URL-style room codes
- Multi-platform builds: linux amd64/arm64/386, darwin amd64/arm64, windows amd64

### Changed

- Moved binary distribution from self-hosted to GitHub Releases

### Fixed

- Windows and i686 compatibility
- Bootstrapping flow for all platforms

By: @ishaan-jindal
