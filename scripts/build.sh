#!/bin/bash

set -e

echo "Cleaning dist directory..."

rm -rf dist
mkdir -p dist

echo "Building Linux amd64..."
GOOS=linux GOARCH=amd64 \
go build -o dist/termchat-linux-amd64 ./cli

echo "Building Linux arm64..."
GOOS=linux GOARCH=arm64 \
go build -o dist/termchat-linux-arm64 ./cli

echo "Building macOS amd64..."
GOOS=darwin GOARCH=amd64 \
go build -o dist/termchat-darwin-amd64 ./cli

echo "Building macOS arm64..."
GOOS=darwin GOARCH=arm64 \
go build -o dist/termchat-darwin-arm64 ./cli

echo "Building Windows amd64..."
GOOS=windows GOARCH=amd64 \
go build -o dist/termchat-windows-amd64.exe ./cli

echo "Building Windows arm64..."
GOOS=windows GOARCH=arm64 \
go build -o dist/termchat-windows-arm64.exe ./cli

echo "Done."
