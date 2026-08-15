# AGENTS.md

## Project

termchat — minimal anonymous terminal chatrooms. A **single Go module** (`module termchat`, no go.work) with four packages:

- `cli/` — Bubble Tea TUI client, argument parsing, LAN host mode (embeds the WebSocket server in-process), LAN discovery
- `server/` — WebSocket server, room management, host succession, `/discover`, `/healthz`
- `api/` — HTTP API (Chi): room bootstrap scripts (embedded via `go:embed`), binary redirects to GitHub Releases, `/api/new`, `/healthz`
- `shared/` — protocol types (`Message`, `UserInfo`, `RoomInfo`), room code validation/generation, discovery constants

## Commands

```bash
just check        # full CI gate: tidy-check, fmt-check, vet, build, test -race
go test -race ./...   # all tests must pass under the race detector
just cross        # cross-compile all 8 release platforms
just server       # run WebSocket server locally (port 8080)
just api          # run API server locally (port 3000)
make build        # official CLI build with version ldflags -> dist/
make install      # packaging (binary, man page, license)
```

## Conventions

- `gofmt` + `go vet` clean; `go mod tidy` before committing. All of this is enforced in CI.
- **Server concurrency** (non-negotiable):
  - `rooms` map only under `roomsMutex` (read AND write).
  - Mutable client fields (nickname, color, typing, last activity) only under `client.mu`.
  - Lock order: `room.Mutex` → `client.mu`, never the reverse.
  - NEVER close `client.Send`; lifecycle uses the idempotent `done` channel (`client.close()`).
  - Broadcasts use `client.trySend()` (non-blocking, shutdown-aware).
- New or changed server behavior MUST come with race-tested integration tests in `server/websocket_test.go` (real WebSocket clients against `handleWebSocket`).
- Rune-safe truncation (`truncateRunes`) — never byte-slice strings for nick/text truncation.
- Server-side input sanitization strips ANSI escapes and control characters.
- Only `message` type is broadcast from clients; all other client frames are ignored.
- Room codes are 4 chars (case-insensitive, A-Z0-9) — changing `RoomCodeLength` in `shared/constants.go` touches every consumer.

## Protocol / flow

- Client connects to `/ws`, first frame MUST be `join` `{nick, room, password}`.
- First joiner becomes host; on host disconnect the next-oldest client succeeds.
- Room is deleted when empty; history capped at 30 messages.
- Rate limit: 5 frames/sec per client; idle clients disconnected after 30 min.
- `termchat discover --online` hits `/discover`; LAN discovery uses a UDP multicast beacon on `224.0.0.167:9999`.
- The API renders `api/scripts/bootstrap.sh` / `.ps1` with `{Room, ApiURL, Version}`; scripts download release binaries from GitHub and exec them. The version is cached from the GitHub API every 5 min.

## CI / release

- `.github/workflows/ci.yml` — PR gate: tidy, gofmt, vet, build, `go test -race`, 8-platform cross-compile.
- Tag `cli-v*` → `.github/workflows/cli.yml`: builds 8 binaries, generates `termchat-checksums.txt`, creates the GitHub Release via `gh`, then calls `aur.yml` (AUR package sync).
- `api.yml` / `websocket.yml` — GHCR images on `main` push, path-filtered; manually dispatchable.
- Secrets: `AUR_SSH_PRIVATE_KEY`. Repo variables: `TERMCHAT_API_URL`, `TERMCHAT_WS_URL` (baked into the CLI via ldflags `-X main.DefaultAPI/DefaultWS`).
- `main` is protected: PR + required checks (`lint-and-test`, `cross-compile`). Never push directly.
- Commit signing is mandatory (`git commit -s -S`).

## Deployment

- `docker-compose.yml`: websocket + api + caddy + watchtower; config via `.env` (see `.env.example`).
- Caddy reverse proxies `/ws*` and `/discover` to websocket:8080, everything else to api:3000; automatic HTTPS on termchat.sacred99.online.
- API env: `API_PORT`, `PUBLIC_API_URL`, `GITHUB_REPO`.

## Gotchas

- Bootstrap scripts live in `api/scripts/` (embedded into the API binary) — not at the repo root.
- The CLI's default server URLs are compile-time baked; the bootstrap flow does not override them.
- CHANGELOG entries for releases must be titled `## [cli-vX.Y.Z]` (the release workflow extracts notes by tag).
- `dist/` is gitignored; never commit binaries.
