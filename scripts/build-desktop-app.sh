#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo "desktop app builds require macOS" >&2; exit 1; }

GO_BIN="${GO_BIN:-go}"
ARCH="${OPENSURGE_DESKTOP_ARCH:-$("$GO_BIN" env GOARCH)}"
case "$ARCH" in
  arm64) GO_ARCH=arm64 ;;
  amd64|x86_64) ARCH=x86_64; GO_ARCH=amd64 ;;
  *) echo "unsupported desktop architecture: $ARCH" >&2; exit 1 ;;
esac

OUTPUT="$ROOT/bin/OpenSurge Desktop Preview.app"
mkdir -p "$OUTPUT/Contents/MacOS" "$OUTPUT/Contents/Resources"
(
  cd "$ROOT/apps/desktop"
  MACOSX_DEPLOYMENT_TARGET=13.0 GOOS=darwin GOARCH="$GO_ARCH" CGO_ENABLED=1 \
    CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=13.0" \
    CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=13.0" \
    "$GO_BIN" build -mod=readonly -trimpath -tags production \
      -o "$OUTPUT/Contents/MacOS/OpenSurgeDesktop" .
)
cp "$ROOT/apps/desktop/Resources/Info.plist" "$OUTPUT/Contents/Info.plist"
/usr/bin/plutil -lint "$OUTPUT/Contents/Info.plist"
/usr/bin/lipo "$OUTPUT/Contents/MacOS/OpenSurgeDesktop" -verify_arch "$ARCH"
/usr/bin/codesign --force --sign - --timestamp=none "$OUTPUT"
/usr/bin/codesign --verify --strict "$OUTPUT"
printf 'Built %s for %s (local ad-hoc signature)\n' "$OUTPUT" "$ARCH"
