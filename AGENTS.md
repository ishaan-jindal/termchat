# AGENTS.md

AI-specific and meta rules for working in this repository.

To get an idea about the project read [README.md](README.md).

## Rule hierarchy

1. The rules in this file (Formatting, Testing, Server concurrency) - non-negotiable.
2. [CONTRIBUTING.md](CONTRIBUTING.md) - canonical coding and workflow rules.

Resolve conflicts upward: AGENTS.md beats CONTRIBUTING.md. Do not restate or
reinterpret project rules locally. Everything not fixed here is discussable -
always ask before guessing.

## Formatting

- Code comments should be no longer than one line, unless they are required
  to cover complex unintuitive logic.
- Never explain previous behaviour in comments.
- Comments are not safeguards, they are informal. An API is safe to use from
  several goroutines because it is mutex-free or confined to one, never
  because a comment says it is.
- Commit messages should be kept as short and to the point as possible (1-3
  lines). Keep the conventional `<type>(<scope>): <description>` format from
  CONTRIBUTING.md.
- ASCII only in code and docs. No em-dashes, arrows, or fancy bullets.
- We do not use the define and test one line `if` syntax, instead splitting
  definition and testing across two lines:

  ```go
  // Avoid
  if err := op(); err != nil {
      return err
  }

  // Prefer
  err := op()
  if err != nil {
      return err
  }
  ```

## Testing

Use `just` commands instead of raw `go` where one exists (see `just --list`).

- `just check` runs everything CI enforces (tidy, gofmt, vet, build, race tests).
- `just test-e2e` runs the end-to-end suite (real server, real CLI networking).
- All tests must pass under the race detector: `go test -race ./...`.
- New or changed server behavior MUST ship with race-tested integration
  tests: `server/websocket_test.go` (real WebSocket clients) and
  `server/bootstrap_test.go` (HTTP bootstrap routes).
- Fuzz targets must keep passing; CI runs them with a bounded fuzztime.
- Capture test output yourself with `just test` / `just test-race`; do not
  guess why a test failed from memory.

## Working in this repo

- Check existing patterns in the codebase before creating new ones.
- In most cases we do not enforce safety by comments, we enforce by code and
  architecture.
- Think through framework/library behavior before coding.
- Keep code direct - no unnecessary intermediate variables; use `_` for
  unused parameters.
- If cycling (same approach, no progress), stop and ask.
- Removing user-visible output or an exported symbol is its own announced
  change, never folded into a cleanup.
- Run long commands (test suites, builds) in the background so the terminal
  stays usable, and report when they exit.
- Never chain edit -> test -> restore in one shell invocation. Interrupted
  or denied mid-chain the edit lands and the restore never runs; keep each
  step separately reversible.

## Server concurrency (non-negotiable)

- `rooms` map only under `roomsMutex` (read AND write).
- Mutable client fields (nickname, color, typing, last activity) only under
  `client.mu`.
- Lock order: `room.Mutex` -> `client.mu`, never the reverse.
- NEVER close `client.Send`; lifecycle uses the idempotent `done` channel
  (`client.close()`).
- Broadcasts use `client.trySend()` (non-blocking, shutdown-aware).

## Protocol / flow

- Client connects to `/ws`, first frame MUST be `join` `{nick, room, password}`.
- First joiner becomes host; on host disconnect the next-oldest client succeeds.
- Room is deleted when empty; history capped at 30 messages.
- Rate limit: 5 frames/sec per client; idle clients disconnected after 30 min.
- `termchat discover --online` hits `/discover`; LAN discovery uses a UDP
  multicast beacon on `224.0.0.167:9999`.
- The server renders `server/scripts/bootstrap.sh` / `.ps1` with
  `{Room, BaseURL, Version}`; scripts download release binaries from GitHub
  and exec them with `--server` (derived from `PUBLIC_BASE_URL`). The CLI
  version is cached from the GitHub API every 5 min.
- Room codes are generated client-side (`shared.GenerateRoomCode()`); the CLI
  no longer calls any HTTP endpoint to create rooms.

## CI / release

- `.github/workflows/ci.yml` - PR gate: tidy, gofmt, vet as a format check,
  `go test -race` and bounded fuzzing as separate checks.
- Tag `cli-v*` -> `.github/workflows/cli.yml`: builds 8 binaries, generates
  `termchat-checksums.txt`, creates the GitHub Release via `gh`, then calls
  `aur.yml` (AUR package sync).
- `websocket.yml` - GHCR image on `main` push, path-filtered; manually
  dispatchable.
- Dependabot keeps `gomod` and `github-actions` dependencies updated;
  dependency review runs on every PR.
- Secrets: `AUR_SSH_PRIVATE_KEY`. Repo variables: `TERMCHAT_WS_URL` (baked
  into the CLI via ldflags `-X main.DefaultWS`).
- `main` is protected: PR + required checks (`gate / format`,
  `gate / test`). Never push directly.
- Commit signing is mandatory (`git commit -s -S`).

## Deployment

- `docker-compose.yml`: websocket + caddy + watchtower; config via `.env`
  (see `.env.example`).
- Caddy reverse proxies everything to websocket:8080; automatic HTTPS on
  termchat.sacred99.online.
- Server env: `WS_HOST`, `WS_PORT`, `PUBLIC_BASE_URL` (bootstrap scripts +
  binary redirects), `GITHUB_REPO`.

## Gotchas

- Bootstrap scripts live in `server/scripts/` (embedded into the server
  binary) - not at the repo root.
- The CLI's default server URL is compile-time baked; the bootstrap flow
  overrides it with `--server` derived from `PUBLIC_BASE_URL`.
- CHANGELOG entries for releases must be titled `## [cli-vX.Y.Z]` (the
  release workflow extracts notes by tag).
- `dist/` is gitignored; never commit binaries.
