# Changelog

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

## [cli-v1.1.0] - 2026-06-04

### Added

- Room passwords (locked/unlocked rooms) with interactive prompt on join
- Host privileges with automatic succession on host disconnect
- LAN room discovery via `termchat discover --local`

### Changed

- Removed emoji rendering from TUI
- Updated documentation and man page for password and host features

## [cli-v1.0.1] - 2026-06-03

No user-facing changes.

## [cli-v1.0.0] - 2026-06-02

### Added

- LAN Host Mode: built-in WebSocket server, room manager, and TUI in one process
- `termchat host` command with auto-generated room codes
- LAN join via `--host` and `--port` flags
- UDP multicast beacon for local room discovery

## [cli-v0.4.4] - 2026-06-02

### Changed

- Extracted shared types into a `shared` module
- Fixed AUR publishing workflow

## [cli-v0.4.3] - 2026-06-01

### Added

- Typing indicator — shows `[...]` next to users currently typing

## [cli-v0.4.2] - 2026-05-31

### Added

- Makefile with build, install, uninstall targets
- Man page (`doc/termchat.1`)
- MIT License

### Changed

- Updated default API and WebSocket URLs

## [cli-v0.4.1] - 2026-05-19

### Fixed

- Mention highlighting now highlights the full message, not just the nickname

## [cli-v0.4.0] - 2026-05-19

### Added

- User list sidebar with colored nicknames, joined timestamps, and typing indicator
- User info broadcast (nickname, color, join time, typing status) from server

### Fixed

- `/color` now immediately broadcasts updated user list to all clients

## [cli-v0.3.0] - 2026-05-17

### Added

- Persistent config (`~/.termchat/config.json`) for nickname and color
- `--version` flag
- Multiline textarea input (Alt+Enter for newline)
- Standalone CLI UX improvements
- Updated API routes and bootstrap scripts for new features

## [cli-v0.2.1] - 2026-05-16

### Added

- Idle user cleanup (30 min timeout)
- Spam prevention (5 messages/second max)

### Fixed

- Mouse scroll support
- Non-command text starting with `/` now sent as normal message

### Changed

- API server periodically refreshes latest CLI version

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
