#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDKROOT="${SDKROOT:-/Library/Developer/CommandLineTools/SDKs/MacOSX14.5.sdk}"
MODULE_CACHE="${CLANG_MODULE_CACHE_PATH:-/private/tmp/opensurge-swift-module-cache}"
OUTPUT="${OPENSURGE_MENUBAR_CHECK_BINARY:-/private/tmp/opensurge-menubar-check}"

swiftc -parse-as-library -sdk "$SDKROOT" -module-cache-path "$MODULE_CACHE" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/APIClient.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/ControlServiceLauncher.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/MenuBarIcon.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/Models.swift" \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/WebGUIURLLauncher.swift" \
  "$ROOT/apps/menubar/Checks/MenuBarChecks.swift" \
  -o "$OUTPUT"
"$OUTPUT"

if grep -Eq '/usr/bin/osascript|bootout[[:space:]]+system/com\.opensurge\.helper' \
  "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/ControlServiceLauncher.swift"; then
  echo "menu bar quit flow must leave the launchd-managed root Helper loaded" >&2
  exit 1
fi

if grep -Fq 'Task.detached' "$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/ControlServiceLauncher.swift"; then
  echo "Control Service wake and bootout must remain serialized without actor reentrancy" >&2
  exit 1
fi

MENU_CONTENT="$ROOT/apps/menubar/Sources/OpenSurgeMenuBar/MenuContentView.swift"
grep -Fq 'let alert = NSAlert()' "$MENU_CONTENT" || {
  echo "menu bar quit confirmation must use a synchronous AppKit alert" >&2
  exit 1
}
grep -Fq 'alert.runModal() == .alertFirstButtonReturn' "$MENU_CONTENT" || {
  echo "menu bar quit action must use the synchronous AppKit alert result" >&2
  exit 1
}
if grep -Fq '.alert(' "$MENU_CONTENT"; then
  echo "menu bar quit action must not depend on SwiftUI alert dismissal state" >&2
  exit 1
fi
if grep -Fq 'ProgressView' "$MENU_CONTENT"; then
  echo "background menu bar polling must not show a periodic loading spinner" >&2
  exit 1
fi
