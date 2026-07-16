#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDKROOT="${SDKROOT:-/Library/Developer/CommandLineTools/SDKs/MacOSX14.5.sdk}"
MODULE_CACHE="${CLANG_MODULE_CACHE_PATH:-/private/tmp/opensurge-swift-module-cache}"
OUTPUT="${OPENSURGE_MENUBAR_CHECK_BINARY:-/private/tmp/opensurge-menubar-check}"

swiftc -parse-as-library -sdk "$SDKROOT" -module-cache-path "$MODULE_CACHE" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/APIClient.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/Models.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/WebGUIURLLauncher.swift" \
  "$ROOT/apps/menubar/Checks/MenuBarChecks.swift" \
  -o "$OUTPUT"
"$OUTPUT"
