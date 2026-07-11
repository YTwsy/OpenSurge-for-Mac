#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE="$ROOT/apps/menubar"
OUTPUT="$ROOT/bin/OpenSurge Menu Bar.app"

SDKROOT="${SDKROOT:-/Library/Developer/CommandLineTools/SDKs/MacOSX14.5.sdk}" \
CLANG_MODULE_CACHE_PATH="${CLANG_MODULE_CACHE_PATH:-/private/tmp/opensurge-swift-module-cache}" \
SWIFTPM_MODULECACHE_OVERRIDE="${SWIFTPM_MODULECACHE_OVERRIDE:-/private/tmp/opensurge-swift-module-cache}" \
swift build --disable-sandbox --package-path "$PACKAGE" --scratch-path /private/tmp/opensurge-menubar-release -c release
rm -rf "$OUTPUT"
mkdir -p "$OUTPUT/Contents/MacOS" "$OUTPUT/Contents/Resources"
cp "$PACKAGE/.build/release/OpenSurgeMenuBar" "$OUTPUT/Contents/MacOS/OpenSurgeMenuBar"
cp "$PACKAGE/Resources/Info.plist" "$OUTPUT/Contents/Info.plist"
printf 'Built %s\n' "$OUTPUT"
