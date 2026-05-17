#!/bin/bash

ROOM="{{.Room}}"
API_URL="{{.ApiURL}}"

OS=$(uname -s)
ARCH=$(uname -m)

# Detect Termux / Android
if [ -n "$TERMUX_VERSION" ]; then
    PLATFORM="android"
fi

case "$OS" in
    Linux)
        if [ -z "$PLATFORM" ]; then
            PLATFORM="linux"
        fi
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

    curl -fsSL "$API_URL/bin/$BINARY" -o "$BINARY_PATH"

    chmod +x "$BINARY_PATH"

    echo "{{.Version}}" > "$VERSION_FILE"
else
    echo "Using cached $BINARY..."
fi

echo "Launching room $ROOM..."

exec "$BINARY_PATH" "$ROOM"
