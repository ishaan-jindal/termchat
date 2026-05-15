#!/bin/bash

ROOM="{{.Room}}"
API_URL="{{.ApiURL}}"
WS_URL="{{.WsURL}}"

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

TMP=$(mktemp)

echo "Downloading $BINARY..."

curl -fsSL "$API_URL/bin/$BINARY" -o "$TMP"

chmod +x "$TMP"

echo "Launching room $ROOM..."

"$TMP" --room "$ROOM" --server "$WS_URL"
