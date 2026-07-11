#!/usr/bin/env bash
set -euo pipefail

PKG="${1:-}"
PROFILE="${OPENSURGE_NOTARY_PROFILE:-}"
[[ -f "$PKG" ]] || { echo "usage: $0 /path/to/OpenSurge.pkg" >&2; exit 1; }
[[ -n "$PROFILE" ]] || { echo "OPENSURGE_NOTARY_PROFILE is required" >&2; exit 1; }
xcrun notarytool submit "$PKG" --keychain-profile "$PROFILE" --wait
xcrun stapler staple "$PKG"
xcrun stapler validate "$PKG"
