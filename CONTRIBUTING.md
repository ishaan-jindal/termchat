# Contributing to termchat

Thank you for your interest in contributing to termchat!

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Making Changes](#making-changes)
- [Style Guidelines](#style-guidelines)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Questions?](#questions)

## Code of Conduct

This project is governed by the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainer.

## Getting Started

1. **Open an Issue** — Before submitting a Pull Request, please open a corresponding issue to discuss your proposed changes, bug fix, or feature. This avoids wasted effort if the change is not a good fit.
2. **Fork the repository** on GitHub.
3. **Clone your fork** to your local machine.
4. **Create a new branch** — Use a descriptive name like `feat/amazing-feature` or `fix/bug-description`.

## Development Setup

### Prerequisites

- **Go 1.26+** — [Download](https://go.dev/dl/)
- **Make** (optional, for convenience targets)

The repository is a single Go module; any `go` command from the repo root
covers all packages (`cli`, `api`, `server`, `shared`).

### Build

```bash
# Build the CLI
go build -o termchat ./cli

# Build the WebSocket server
go build -o termchat-server ./server/cmd/server

# Build the API server
go build -o termchat-api ./api
```

### Run

```bash
# Run the CLI locally (connects to production server by default)
go run ./cli

# Run a local host session (embeds server + CLI)
go run ./cli host
```

## Project Structure

```
termchat/
  cli/          — Terminal UI client (Bubble Tea)
  server/       — WebSocket server + room management
  api/          — HTTP API server (Chi router)
  api/scripts/  — Bootstrap installers (embedded into the API binary)
  shared/       — Shared types, validation, constants
  caddy/        — Reverse proxy config
```

## Making Changes

### What to Work On

Check the [open issues](https://github.com/ishaan-jindal/termchat/issues) for `good first issue` or `help wanted` labels. If you want to work on something not listed, open an issue first.

### Commit Messages

Write clear, concise commit messages that explain the "why" behind your changes:

```
feat: add /users command to list room members inline
fix: handle empty room code gracefully on join
refactor: extract broadcast logic into separate function
```

## Style Guidelines

- Run `gofmt` on all Go files before committing.
- Run `go vet ./...` and fix any warnings.
- Follow standard Go conventions from [Effective Go](https://go.dev/doc/effective_go).
- Use meaningful variable names — avoid single-letter names outside short loops.
- Keep functions focused and reasonably sized; extract helpers when a function exceeds ~60 lines.
- Handle errors explicitly — never use `_` to ignore an error unless you have a good reason.

## Testing

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests for a specific package
go test ./server/...

# View coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

We aim for good test coverage, especially on:
- `server/` — broadcast logic, client cleanup, input sanitization
- `shared/` — validation functions, protocol helpers
- `cli/` — argument parsing

## Pull Request Process

1. Ensure your code compiles: `go build ./...`
2. Run `gofmt` and `go vet` with no errors.
3. Run `go test ./...` and ensure all tests pass. Add tests for new functionality.
4. Update the README or documentation if your change impacts usage.
5. Reference the issue number in your PR description (e.g., `Fixes #123`).
6. Provide a clear, concise description of your changes.
7. Wait for feedback and address any requested changes.

## Questions?

Open a [discussion](https://github.com/ishaan-jindal/termchat/discussions) or ask in the issue you're working on.
