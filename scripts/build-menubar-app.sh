#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="$ROOT/apps/menubar"
OUTPUT="$ROOT/bin/OpenSurge Menu Bar.app"
SCRATCH="${OPENSURGE_SWIFT_SCRATCH:-/private/tmp/opensurge-menubar-release}"
APP_ICON_SOURCE="$PACKAGE/Resources/OpenSurgeAppIcon.png"
MENU_BAR_ICON_SOURCE="$PACKAGE/Resources/OpenSurgeMenuBarIcon.png"
APP_ICONSET="$SCRATCH/OpenSurgeAppIcon.iconset"
VERSION="${OPENSURGE_VERSION:-0.1.0}"
BUILD_NUMBER="${OPENSURGE_BUILD_NUMBER:-1}"
ARCH="${OPENSURGE_APP_ARCH:-}"

[[ "$VERSION" =~ ^[0-9]+([.][0-9]+){1,2}$ ]] || { echo "invalid OpenSurge app version: $VERSION" >&2; exit 1; }
[[ "$BUILD_NUMBER" =~ ^[0-9]+$ ]] || { echo "invalid OpenSurge app build number: $BUILD_NUMBER" >&2; exit 1; }
[[ -s "$APP_ICON_SOURCE" && -s "$MENU_BAR_ICON_SOURCE" ]] || { echo "OpenSurge icon assets are missing" >&2; exit 1; }
if [[ -z "$ARCH" && -x "$ROOT/bin/omg" ]]; then
  ARCH="$(/usr/bin/lipo -archs "$ROOT/bin/omg")"
fi
case "$ARCH" in
  arm64|x86_64) ;;
  *) echo "unable to determine a single supported OpenSurge app architecture: ${ARCH:-unset}" >&2; exit 1 ;;
esac

SDKROOT="${SDKROOT:-/Library/Developer/CommandLineTools/SDKs/MacOSX14.5.sdk}" \
CLANG_MODULE_CACHE_PATH="${CLANG_MODULE_CACHE_PATH:-/private/tmp/opensurge-swift-module-cache}" \
SWIFTPM_MODULECACHE_OVERRIDE="${SWIFTPM_MODULECACHE_OVERRIDE:-/private/tmp/opensurge-swift-module-cache}" \
swift build --disable-sandbox --package-path "$PACKAGE" --scratch-path "$SCRATCH" -c release --arch "$ARCH"
rm -rf "$OUTPUT"
mkdir -p "$OUTPUT/Contents/MacOS" "$OUTPUT/Contents/Resources"
cp "$SCRATCH/$ARCH-apple-macosx/release/OpenSurgeMenuBar" "$OUTPUT/Contents/MacOS/OpenSurgeMenuBar"
cp "$PACKAGE/Resources/Info.plist" "$OUTPUT/Contents/Info.plist"
cp "$MENU_BAR_ICON_SOURCE" "$OUTPUT/Contents/Resources/OpenSurgeMenuBarIcon.png"
rm -rf "$APP_ICONSET"
mkdir -p "$APP_ICONSET"
for icon_spec in \
  "16:icon_16x16.png" "32:icon_16x16@2x.png" \
  "32:icon_32x32.png" "64:icon_32x32@2x.png" \
  "128:icon_128x128.png" "256:icon_128x128@2x.png" \
  "256:icon_256x256.png" "512:icon_256x256@2x.png" \
  "512:icon_512x512.png" "1024:icon_512x512@2x.png"; do
  icon_size="${icon_spec%%:*}"
  icon_name="${icon_spec#*:}"
  /usr/bin/sips -z "$icon_size" "$icon_size" "$APP_ICON_SOURCE" --out "$APP_ICONSET/$icon_name" >/dev/null
done
/usr/bin/iconutil -c icns "$APP_ICONSET" -o "$OUTPUT/Contents/Resources/OpenSurgeAppIcon.icns"
/usr/bin/plutil -replace CFBundleShortVersionString -string "$VERSION" "$OUTPUT/Contents/Info.plist"
/usr/bin/plutil -replace CFBundleVersion -string "$BUILD_NUMBER" "$OUTPUT/Contents/Info.plist"
/usr/bin/lipo "$OUTPUT/Contents/MacOS/OpenSurgeMenuBar" -verify_arch "$ARCH"
printf 'Built %s version %s (%s) for %s\n' "$OUTPUT" "$VERSION" "$BUILD_NUMBER" "$ARCH"
