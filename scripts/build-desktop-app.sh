#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo "desktop app builds require macOS" >&2; exit 1; }

GO_BIN="${GO_BIN:-go}"
VARIANT="${1:-preview}"
case "$VARIANT" in
  preview) APP_NAME="OpenSurge Desktop Preview"; BUNDLE_ID=com.opensurge.desktop.preview ;;
  production) APP_NAME=OpenSurge; BUNDLE_ID=com.opensurge.menubar ;;
  *) echo "usage: $0 [preview|production]" >&2; exit 2 ;;
esac
RELEASE_TAG="${OPENSURGE_RELEASE_TAG:-v0.3.0-rc.2}"
[[ "$RELEASE_TAG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$ ]] || { echo "invalid desktop release tag" >&2; exit 1; }
RELEASE_VERSION="${RELEASE_TAG#v}"
RELEASE_VERSION="${RELEASE_VERSION%%[-+]*}"
VERSION="${OPENSURGE_VERSION:-$RELEASE_VERSION}"
BUILD_NUMBER="${OPENSURGE_BUILD_NUMBER:-1}"
[[ "$VERSION" == "$RELEASE_VERSION" ]] || { echo "desktop release tag does not match package version" >&2; exit 1; }
[[ "$BUILD_NUMBER" =~ ^[0-9]+$ ]] || { echo "invalid desktop build number" >&2; exit 1; }
ARCH="${OPENSURGE_DESKTOP_ARCH:-$("$GO_BIN" env GOARCH)}"
if [[ "$VARIANT" == production ]]; then ARCH="${OPENSURGE_APP_ARCH:-$ARCH}"; fi
case "$ARCH" in
  arm64) GO_ARCH=arm64 ;;
  amd64|x86_64) ARCH=x86_64; GO_ARCH=amd64 ;;
  *) echo "unsupported desktop architecture: $ARCH" >&2; exit 1 ;;
esac

OUTPUT="$ROOT/bin/$APP_NAME.app"
# Never replace a mapped, ad-hoc-signed executable underneath a running host.
if /usr/sbin/lsof -nP -a -d txt -c OpenSurge -Fn 2>/dev/null | /usr/bin/awk -v prefix="n$OUTPUT/Contents/MacOS/" 'index($0, prefix) == 1 { found=1 } END { exit !found }'; then
  echo "quit the running desktop App before rebuilding: $OUTPUT" >&2
  exit 1
fi
mkdir -p "$ROOT/bin"
STAGING="$(mktemp -d "$ROOT/bin/.desktop-$VARIANT.XXXXXX")"
trap 'rm -rf "$STAGING"' EXIT
BUNDLE="$STAGING/$APP_NAME.app"
mkdir -p "$BUNDLE/Contents/MacOS" "$BUNDLE/Contents/Resources"
(
  cd "$ROOT/apps/desktop"
  MACOSX_DEPLOYMENT_TARGET=13.0 GOOS=darwin GOARCH="$GO_ARCH" CGO_ENABLED=1 \
    CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=13.0" \
    CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=13.0" \
    "$GO_BIN" build -mod=readonly -trimpath -tags production,private_mac_apis -ldflags "-extldflags=-mmacosx-version-min=13.0 -X main.releaseTag=$RELEASE_TAG -X main.bundleIdentifier=$BUNDLE_ID" \
      -o "$BUNDLE/Contents/MacOS/OpenSurgeDesktop" .
)
cp "$ROOT/apps/desktop/Resources/Info.plist" "$BUNDLE/Contents/Info.plist"
PLIST="$BUNDLE/Contents/Info.plist"
/usr/bin/plutil -replace CFBundleIdentifier -string "$BUNDLE_ID" "$PLIST"
/usr/bin/plutil -replace CFBundleName -string "$APP_NAME" "$PLIST"
/usr/bin/plutil -replace CFBundleDisplayName -string "$APP_NAME" "$PLIST"
/usr/bin/plutil -insert OpenSurgeReleaseTag -string "$RELEASE_TAG" "$PLIST"
/usr/bin/plutil -insert OpenSurgeReleaseCodename -string "$(python3 "$ROOT/scripts/release-codename.py" "$RELEASE_TAG")" "$PLIST"
/usr/bin/plutil -replace CFBundleShortVersionString -string "$VERSION" "$PLIST"
/usr/bin/plutil -replace CFBundleVersion -string "$BUILD_NUMBER" "$PLIST"
ICONSET="$STAGING/OpenSurgeAppIcon.iconset"
mkdir -p "$ICONSET"
for icon_spec in \
  "16:icon_16x16.png" "32:icon_16x16@2x.png" \
  "32:icon_32x32.png" "64:icon_32x32@2x.png" \
  "128:icon_128x128.png" "256:icon_128x128@2x.png" \
  "256:icon_256x256.png" "512:icon_256x256@2x.png" \
  "512:icon_512x512.png" "1024:icon_512x512@2x.png"; do
  /usr/bin/sips -z "${icon_spec%%:*}" "${icon_spec%%:*}" \
    "$ROOT/apps/desktop/Resources/OpenSurgeAppIcon.png" --out "$ICONSET/${icon_spec#*:}" >/dev/null
done
/usr/bin/iconutil -c icns "$ICONSET" -o "$BUNDLE/Contents/Resources/OpenSurgeAppIcon.icns"
/usr/bin/codesign --force --sign - --timestamp=none "$BUNDLE"
"$ROOT/scripts/verify-desktop-app.sh" "$BUNDLE" "$ARCH" "$VARIANT" "$VERSION" "$BUILD_NUMBER" "$RELEASE_TAG"
# Rebuild from a clean bundle so an old Swift executable cannot enter the PKG.
rm -rf "$OUTPUT"
mv "$BUNDLE" "$OUTPUT"
printf 'Built %s for %s (local ad-hoc signature)\n' "$OUTPUT" "$ARCH"
