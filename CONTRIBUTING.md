# Contributing to termchat

Thank you for your interest in contributing! This document outlines the
conventions and practices we follow. AI agents working in this repo must also
follow [AGENTS.md](AGENTS.md), whose rules take precedence.

## Philosophy

**KISS** - Keep It Simple, Stupid. As well as "boring" code. These are the
guiding principles for all work.

- Prefer shallow package structure over deep nesting
- Direct code over abstractions
- Working software over perfect architecture
- Simple solutions over clever ones
- No non-ASCII characters in code and docs

## Project Structure

```
termchat/
  cli/              # Terminal UI client (Bubble Tea)
  server/           # WebSocket server, rooms, host succession
  server/cmd/server # Server binary entry point
  server/scripts/   # Bootstrap installers (embedded via go:embed)
  shared/           # Protocol types, validation, room codes
  caddy/            # Reverse proxy config
```

**Package Guidelines**:

- `cli/` - TUI, argument parsing, LAN host mode (embeds the server in-process)
- `server/` - WebSocket protocol, room lifecycle, bootstrap HTTP routes
- `shared/` - Types both binaries use; change it and both need testing
- There is no separate API server; the server serves everything

**Don't create**:

- Deep nesting like `pkg/application/container/`
- Abstraction layers "for future flexibility"

## Build and Test Commands

Use `just` (see `just --list`); it mirrors what CI enforces:

```bash
just --list       # Show all available commands
just pre-commit   # Run before committing (tidy, fmt, vet, build, race tests)
just check        # Same as pre-commit
just test-race    # Tests under the race detector
just test-e2e     # End-to-end suite (real server, real CLI networking)
just cross        # Cross-compile all 8 release platforms
just server       # Run the WebSocket server locally (port 8080)
```

Prerequisites: Go 1.26+, `just` (recommended), `make` (for packaging only).

## Code Style

### Naming

Prefer Go-style concise names over verbose ones:

| Prefer    | Avoid            |
| --------- | ---------------- |
| `trySend` | `AttemptSend`    |
| `mu`      | `wellKnownMu`    |
| `nick`    | `nicknameString` |
| `err`     | `errorResult`    |

### Comments

- All exported functions and types need doc comments ending with a period
- No misleading comments - if code is self-explanatory, don't comment
- Comments are code: keep them as short as what they explain, and delete
  them when the code already says it

### Unused parameters

Use `_` for unused parameters rather than ignoring in the function body:

```go
// Preferred
func (t *logTerminal) Read(_ []byte) (int, error) {

// Avoid
func (t *logTerminal) Read(p []byte) (int, error) {
    _ = p
```

### Error handling

Return errors, don't panic:

```go
if err != nil {
    return fmt.Errorf("joining room %s: %w", name, err)
}
```

### Commit messages

Use conventional commit format with a scope:

```
<type>(<scope>): <description>
```

Types: `fix`, `feat`, `refactor`, `test`, `docs`, `ci`, `chore`.

Scopes: `cli`, `server`, `shared`, `docs`, `ci`.

Keep the message to 1-3 lines. Every commit must be signed off and
GPG/SSH-signed: `git commit -s -S`. Unsigned commits will not pass CI.

### Changelog

[CHANGELOG.md](CHANGELOG.md) follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Add the entry in the same commit as the change, under the unreleased heading,
in `Added`/`Changed`/`Fixed`/`Removed`.

An entry is needed when a user can observe the difference:

- behavior, CLI flags, or command output
- a bug they could have hit, even when the cause was internal
- a change to the WebSocket protocol or bootstrap flow

No entry for contributor docs (`AGENTS.md`, `CONTRIBUTING.md`), CI/CD
workflows, dev tooling (`justfile`), tests, or refactors with no observable
difference.

Write what changed for the reader, not what you did to the code - "room
passwords no longer leak into history", not "added a filter to the
broadcaster".

## Testing

Tests are categorized:

- **Unit** - pure functions and helpers (sanitize, truncate, validation)
- **Integration** - real WebSocket clients and HTTP requests against the
  handlers (`server/websocket_test.go`, `server/bootstrap_test.go`)
- **End-to-end** - the real server binary + the CLI's real networking layer
  (`cli/e2e_test.go`), including a SIGTERM graceful-shutdown test
- **Fuzz** - `Fuzz*` targets in each package; CI runs them with a bounded
  fuzztime

All tests must pass under the race detector. New or changed server behavior
MUST ship with race-tested integration tests. If a fuzz target finds a
crash, fix the code, not the test.

## Pull Request Process

`main` is protected: direct pushes are blocked, and every PR must pass the
required checks (`gate / format` and `gate / test`).

1. Ensure your code compiles: `go build ./...`
2. Run `just pre-commit` and ensure everything passes. Add tests for new
   functionality.
3. Update the README or documentation if your change impacts usage.
4. Reference the issue number in your PR description (e.g., `Fixes #123`).
5. Provide a clear, concise description of your changes.
6. Wait for feedback and address any requested changes.

## Questions?

Open a [discussion](https://github.com/ishaan-jindal/termchat/discussions)
or ask in the issue you're working on.
