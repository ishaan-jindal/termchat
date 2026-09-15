#!/bin/bash

ROOM="{{.Room}}"
BASE_URL="{{.BaseURL}}"
WS_URL="${BASE_URL/http/ws}/ws"

OS=$(uname -s)
ARCH=$(uname -m)

# Termux is served by the termchat-mobile app.
if [ -n "$TERMUX_VERSION" ]; then
    echo "Termux is not supported; use the termchat-mobile app:"
    echo "https://github.com/ishaan-jindal/termchat-mobile"
    exit 1
fi

case "$OS" in
    Linux)
        PLATFORM="linux"
        ;;

    Darwin)
        PLATFORM="darwin"
        ;;

    *)
        echo "Unsupported OS"
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;

    i386|i686)
        ARCH="386"
        ;;

    arm64|aarch64)
        ARCH="arm64"
        ;;

    *)
        echo "Unsupported architecture"
        exit 1
        ;;
esac

BINARY="termchat-$PLATFORM-$ARCH"

CACHE_DIR="$HOME/.termchat"

mkdir -p "$CACHE_DIR"

BINARY_PATH="$CACHE_DIR/$BINARY"
VERSION_FILE="$CACHE_DIR/version"

if [ ! -f "$BINARY_PATH" ] || \
   [ ! -f "$VERSION_FILE" ] || \
   [ "$(cat "$VERSION_FILE")" != "{{.Version}}" ]; then

    echo "Downloading $BINARY..."

    curl -fsSL "$BASE_URL/bin/$BINARY" -o "$BINARY_PATH"

    chmod +x "$BINARY_PATH"

    echo "{{.Version}}" > "$VERSION_FILE"
else
    echo "Using cached $BINARY..."
fi

echo "Launching room $ROOM..."

exec "$BINARY_PATH" "$ROOM" --server "$WS_URL"
