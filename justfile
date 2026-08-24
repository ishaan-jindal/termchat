set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# List available recipes
default:
    @just --list

# Build the CLI binary to dist/
build:
    mkdir -p dist
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/termchat ./cli

# Cross-compile the CLI for all release platforms (linux/darwin/windows/android)
cross:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    for target in linux/amd64 linux/arm64 linux/386 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 android/arm64; do
        os="${target%/*}"
        arch="${target#*/}"
        ext=""
        [ "$os" = "windows" ] && ext=".exe"
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "dist/termchat-$os-$arch$ext" ./cli
        echo "built termchat-$os-$arch$ext"
    done

# Build and stage npm packages locally for one-time trusted publishing setup
npm-bootstrap version:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist/npm
    for target in linux/amd64 linux/arm64 linux/386 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
        os="${target%/*}"
        arch="${target#*/}"
        ext=""
        [ "$os" = "windows" ] && ext=".exe"
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
            -ldflags "-s -w -X main.Version=cli-v{{ version }} -X main.DefaultWS=wss://termchat.sacred99.online/ws" \
            -o "dist/npm/termchat-$os-$arch$ext" ./cli
    done
    ./npm/stage.sh dist/npm "{{ version }}" dist/npm-pkgs
    echo ""
    echo "Staged in dist/npm-pkgs. First-time publish (claims the names):"
    echo "  Publishing requires 2FA: enable TOTP at npmjs.com and use"
    echo "  'npm login' sessions, or set a granular token with bypass-2fa."
    echo "  1. npm login"
    for dir in dist/npm-pkgs/platforms/*/; do
        echo "  2. (cd '$dir' && npm publish --access public)"
    done
    echo "  3. (cd dist/npm-pkgs/root && npm publish --access public)"
    echo "  4. On npmjs.com, give each package a Trusted Publisher:"
    echo "     ishaan-jindal/termchat / npm.yml / allow npm publish,"
    echo "     then set Publishing access to 'disallow tokens'"

# Run the CLI in dev mode
run *args:
    go run ./cli {{ args }}

# Run the WebSocket server locally (port 8080)
server:
    WS_PORT=8080 go run ./server/cmd/server

# Format all Go code in place
fmt:
    gofmt -w .

# Check that all Go code is formatted
fmt-check:
    @files="$(gofmt -l .)"; \
    if [ -n "$files" ]; then \
        echo "unformatted files:"; \
        echo "$files"; \
        exit 1; \
    fi

# Run go vet
vet:
    go vet ./...

# Tidy go.mod / go.sum in place
tidy:
    go mod tidy

# Check that go.mod / go.sum are tidy
tidy-check:
    @if ! go mod tidy -diff > /tmp/tidy.diff 2>&1; then \
        echo "go.mod or go.sum is not tidy. Run 'just tidy' and commit the result."; \
        cat /tmp/tidy.diff; \
        exit 1; \
    fi

# Run tests
test:
    go test ./...

# Run tests with the race detector
test-race:
    go test -race ./...

# Full CI gate: tidy, fmt, vet, build, race tests
check: tidy-check fmt-check vet build test-race

# Run everything CI runs, before committing
pre-commit: check

# Run the end-to-end tests only (real server, real CLI networking)
test-e2e:
    go test -race -run 'TestE2E' ./cli/

# Generate and open a coverage report
cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out
    rm -f coverage.out

# Build the server Docker image locally
docker:
    docker build -f Dockerfile.server -t termchat-server:dev .

# Start the full deployment stack (docker compose)
compose-up:
    docker compose up -d

# Stop the full deployment stack
compose-down:
    docker compose down
