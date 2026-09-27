#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo "desktop app builds require macOS" >&2; exit 1; }

GO_BIN="${GO_BIN:-go}"
RELEASE_TAG="${OPENSURGE_RELEASE_TAG:-v0.2.4-next}"
[[ "$RELEASE_TAG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$ ]] || { echo "invalid desktop release tag" >&2; exit 1; }
RELEASE_VERSION="${RELEASE_TAG#v}"
RELEASE_VERSION="${RELEASE_VERSION%%[-+]*}"
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
    "$GO_BIN" build -mod=readonly -trimpath -tags production -ldflags "-extldflags=-mmacosx-version-min=13.0 -X main.releaseTag=$RELEASE_TAG" \
      -o "$OUTPUT/Contents/MacOS/OpenSurgeDesktop" .
)
cp "$ROOT/apps/desktop/Resources/Info.plist" "$OUTPUT/Contents/Info.plist"
/usr/bin/plutil -insert OpenSurgeReleaseTag -string "$RELEASE_TAG" "$OUTPUT/Contents/Info.plist"
/usr/bin/plutil -replace CFBundleShortVersionString -string "$RELEASE_VERSION" "$OUTPUT/Contents/Info.plist"
ICONSET="$ROOT/bin/desktop-icons/OpenSurgeAppIcon.iconset"
mkdir -p "$ICONSET"
for icon_spec in \
  "16:icon_16x16.png" "32:icon_16x16@2x.png" \
  "32:icon_32x32.png" "64:icon_32x32@2x.png" \
  "128:icon_128x128.png" "256:icon_128x128@2x.png" \
  "256:icon_256x256.png" "512:icon_256x256@2x.png" \
  "512:icon_512x512.png" "1024:icon_512x512@2x.png"; do
  /usr/bin/sips -z "${icon_spec%%:*}" "${icon_spec%%:*}" \
    "$ROOT/apps/menubar/Resources/OpenSurgeAppIcon.png" --out "$ICONSET/${icon_spec#*:}" >/dev/null
done
/usr/bin/iconutil -c icns "$ICONSET" -o "$OUTPUT/Contents/Resources/OpenSurgeAppIcon.icns"
/usr/bin/plutil -lint "$OUTPUT/Contents/Info.plist"
/usr/bin/lipo "$OUTPUT/Contents/MacOS/OpenSurgeDesktop" -verify_arch "$ARCH"
/usr/bin/codesign --force --sign - --timestamp=none "$OUTPUT"
/usr/bin/codesign --verify --strict "$OUTPUT"
printf 'Built %s for %s (local ad-hoc signature)\n' "$OUTPUT" "$ARCH"
