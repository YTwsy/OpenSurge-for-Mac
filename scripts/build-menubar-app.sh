#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="$ROOT/apps/menubar"
OUTPUT="$ROOT/bin/OpenSurge Menu Bar.app"
SCRATCH="${OPENSURGE_SWIFT_SCRATCH:-/private/tmp/opensurge-menubar-release}"
VERSION="${OPENSURGE_VERSION:-0.1.0}"
BUILD_NUMBER="${OPENSURGE_BUILD_NUMBER:-1}"
ARCH="${OPENSURGE_APP_ARCH:-}"

[[ "$VERSION" =~ ^[0-9]+([.][0-9]+){1,2}$ ]] || { echo "invalid OpenSurge app version: $VERSION" >&2; exit 1; }
[[ "$BUILD_NUMBER" =~ ^[0-9]+$ ]] || { echo "invalid OpenSurge app build number: $BUILD_NUMBER" >&2; exit 1; }
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
/usr/bin/plutil -replace CFBundleShortVersionString -string "$VERSION" "$OUTPUT/Contents/Info.plist"
/usr/bin/plutil -replace CFBundleVersion -string "$BUILD_NUMBER" "$OUTPUT/Contents/Info.plist"
/usr/bin/lipo "$OUTPUT/Contents/MacOS/OpenSurgeMenuBar" -verify_arch "$ARCH"
printf 'Built %s version %s (%s) for %s\n' "$OUTPUT" "$VERSION" "$BUILD_NUMBER" "$ARCH"
