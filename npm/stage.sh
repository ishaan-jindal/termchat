#!/usr/bin/env bash
# Stage npm package directories for @sacredcat/termchat.
#
# Usage: stage.sh <dist-dir> <version> <out-dir>
#
# Expects release-named binaries in <dist-dir>: termchat-<goos>-<goarch>[.exe].
# Creates <out-dir>/root and <out-dir>/platforms/<target>, each ready for
# "npm publish --access public --provenance". Used by .github/workflows/npm.yml
# and by "just npm-bootstrap".

set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <dist-dir> <version> <out-dir>" >&2
  exit 1
fi

DIST="$1"
VERSION="$2"
OUT="$3"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

REPO="https://github.com/ishaan-jindal/termchat"
REPO_URL="git+${REPO}.git"

# npm suffix|npm os|npm cpu|goos|goarch|binary extension
targets=(
  "linux-x64|linux|x64|linux|amd64|"
  "linux-arm64|linux|arm64|linux|arm64|"
  "linux-ia32|linux|ia32|linux|386|"
  "darwin-x64|darwin|x64|darwin|amd64|"
  "darwin-arm64|darwin|arm64|darwin|arm64|"
  "win32-x64|win32|x64|windows|amd64|.exe"
  "win32-arm64|win32|arm64|windows|arm64|.exe"
)

case "$VERSION" in
  ""|*[!0-9.]*) echo "error: invalid version '$VERSION'" >&2; exit 1 ;;
esac

rm -rf "$OUT"
mkdir -p "$OUT/platforms"

deps=""
published=""

for entry in "${targets[@]}"; do
  IFS="|" read -r suffix os cpu goos goarch ext <<< "$entry"

  name="@sacredcat/termchat-$suffix"
  src="$DIST/termchat-$goos-$goarch$ext"
  if [ ! -f "$src" ]; then
    echo "error: missing binary $src" >&2
    exit 1
  fi

  dir="$OUT/platforms/$suffix"
  mkdir -p "$dir"

  exe="termchat$ext"
  cp "$src" "$dir/$exe"
  chmod 755 "$dir/$exe"

  cat > "$dir/package.json" <<EOF
{
  "name": "$name",
  "version": "$VERSION",
  "description": "termchat CLI binary for $suffix",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "$REPO_URL"
  },
  "os": ["$os"],
  "cpu": ["$cpu"],
  "files": ["$exe"]
}
EOF

  deps="${deps}    \"${name}\": \"${VERSION}\",
"
  published="$published  $name@$VERSION
"
done

root="$OUT/root"
mkdir -p "$root/bin"

cp "$SCRIPT_DIR/bin/termchat.js" "$root/bin/"
cp "$SCRIPT_DIR/README.md" "$root/"
cp "$SCRIPT_DIR/../LICENSE" "$root/"
chmod 755 "$root/bin/termchat.js"

# Strip the trailing newline and comma from the dependency list.
deps="${deps%$'\n'}"
deps="${deps%,}"

cat > "$root/package.json" <<EOF
{
  "name": "@sacredcat/termchat",
  "version": "$VERSION",
  "description": "Minimal anonymous terminal chatrooms in your terminal",
  "license": "MIT",
  "author": "Ishaan Jindal",
  "homepage": "$REPO#readme",
  "bugs": "$REPO/issues",
  "repository": {
    "type": "git",
    "url": "$REPO_URL"
  },
  "keywords": [
    "terminal",
    "chat",
    "websocket",
    "cli",
    "tui"
  ],
  "bin": {
    "termchat": "bin/termchat.js"
  },
  "files": [
    "bin/",
    "README.md",
    "LICENSE"
  ],
  "engines": {
    "node": ">=18"
  },
  "optionalDependencies": {
$deps
  }
}
EOF

echo "staged packages for termchat@$VERSION:"
printf '%s' "$published"
echo "  @sacredcat/termchat@$VERSION (root)"
