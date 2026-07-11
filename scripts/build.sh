#!/usr/bin/env bash
# Cross-compile holonet for all release targets into dist/. Builds the frontend
# first so it is embedded via go:embed. CGO stays disabled for clean static
# cross-compilation (pure-Go SQLite).
set -euo pipefail

VERSION="${VERSION:-dev}"
LDFLAGS="-s -w -X github.com/t0mer/holonet/internal/version.Version=${VERSION}"
OUT="dist"
BIN="holonet"
PKG="./cmd/holonet"

rm -rf "$OUT"
mkdir -p "$OUT"

# Build the frontend unless a prebuilt dist already exists (CI builds it in a
# separate step and this can be skipped with SKIP_FRONTEND=1).
if [ "${SKIP_FRONTEND:-0}" != "1" ] && [ -d web ]; then
  echo "==> building frontend"
  ( cd web && npm ci && npm run build )
fi

# OS/ARCH[/ARM] targets (release-action guideline full matrix).
targets=(
  "linux amd64"
  "linux arm64"
  "linux arm 7"
  "linux arm 6"
  "linux 386"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

for t in "${targets[@]}"; do
  read -r GOOS GOARCH GOARM <<<"$t"
  name="${BIN}-${GOOS}-${GOARCH}"
  [ -n "${GOARM:-}" ] && name="${name}v${GOARM}"
  ext=""
  [ "$GOOS" = "windows" ] && ext=".exe"
  echo "==> $GOOS/$GOARCH${GOARM:+v$GOARM}"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOARM="${GOARM:-}" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/${name}${ext}" "$PKG"
done

echo "==> built:"
ls -1 "$OUT"
