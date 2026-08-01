#!/usr/bin/env bash
set -euo pipefail

PKG="${1:-}"
EXPECTED_VERSION="${2:-}"
EXPECTED_ARCH="${3:-arm64}"
EXPECTED_MINIMUM_MACOS="${4:-13.0}"

if [[ ! -f "$PKG" || -z "$EXPECTED_VERSION" || -z "$EXPECTED_ARCH" ]]; then
  echo "usage: $0 /path/to/OpenSurge.pkg VERSION [ARCH]" >&2
  exit 2
fi

signature="$(pkgutil --check-signature "$PKG" 2>&1 || true)"
printf '%s\n' "$signature"
grep -Fq 'Status: no signature' <<<"$signature" || {
  echo "expected an explicitly unsigned installer package" >&2
  exit 1
}

payload_files="$(pkgutil --payload-files "$PKG")"
grep -Fxq './Applications/OpenSurge.app' <<<"$payload_files" || {
  echo "OpenSurge app is missing from the installer payload" >&2
  exit 1
}
if grep -Fq './Applications/OpenSurge Menu Bar.app' <<<"$payload_files"; then
  echo "legacy menu bar app name must not remain in the installer payload" >&2
  exit 1
fi
grep -Fxq './Library/PrivilegedHelperTools/com.opensurge.helper' <<<"$payload_files" || {
  echo "privileged helper is missing from the installer payload" >&2
  exit 1
}

work_dir="$(mktemp -d "${TMPDIR:-/private/tmp}/opensurge-pkg-verify.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
expanded="$work_dir/expanded"
payload="$work_dir/payload"
pkgutil --expand "$PKG" "$expanded"

package_info="$expanded/PackageInfo"
grep -Fq "version=\"$EXPECTED_VERSION\"" "$package_info" || {
  echo "pkg receipt version does not match $EXPECTED_VERSION" >&2
  exit 1
}
grep -Fq 'install-location="/"' "$package_info" || {
  echo "pkg install location is not /" >&2
  exit 1
}
grep -Fq 'relocatable="false"' "$package_info" || {
  echo "menu bar app is not marked non-relocatable" >&2
  exit 1
}
grep -Fq "CFBundleShortVersionString=\"$EXPECTED_VERSION\"" "$package_info" || {
  echo "menu bar bundle version does not match $EXPECTED_VERSION" >&2
  exit 1
}

mkdir -p "$payload"
(cd "$payload" && gzip -dc "$expanded/Payload" | cpio -idm --quiet)

executables=(
  "$payload/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeMenuBar"
  "$payload/Library/Application Support/OpenSurge/bin/omg"
  "$payload/Library/Application Support/OpenSurge/bin/mihomo"
  "$payload/Library/Application Support/OpenSurge/bin/opensurge-network"
  "$payload/Library/Application Support/OpenSurge/bin/opensurge-install-config"
  "$payload/Library/Application Support/OpenSurge/bin/dnsmasq"
  "$payload/Library/Application Support/OpenSurge/share/opensurge-control"
  "$payload/Library/PrivilegedHelperTools/com.opensurge.helper"
)
for bundle_name_key in CFBundleName CFBundleDisplayName; do
  [[ "$(/usr/libexec/PlistBuddy -c "Print :$bundle_name_key" "$payload/Applications/OpenSurge.app/Contents/Info.plist")" == "OpenSurge" ]] || {
    echo "packaged app $bundle_name_key must use the OpenSurge product name" >&2
    exit 1
  }
done
version_not_newer_than() {
  local actual=$1 maximum=$2
  local actual_major=${actual%%.*} actual_minor=${actual#*.}
  local maximum_major=${maximum%%.*} maximum_minor=${maximum#*.}
  ((actual_major < maximum_major || (actual_major == maximum_major && actual_minor <= maximum_minor)))
}
for executable in "${executables[@]}"; do
  [[ -x "$executable" ]] || {
    echo "packaged executable is missing: $executable" >&2
    exit 1
  }
  /usr/bin/lipo "$executable" -verify_arch "$EXPECTED_ARCH"
  minimum_macos="$(xcrun vtool -show-build "$executable" | awk '/minos/ { print $2; exit }')"
  [[ -n "$minimum_macos" ]] && version_not_newer_than "$minimum_macos" "$EXPECTED_MINIMUM_MACOS" || {
    echo "packaged executable requires macOS $minimum_macos, newer than $EXPECTED_MINIMUM_MACOS: $executable" >&2
    exit 1
  }
done

echo "Verified unsigned OpenSurge $EXPECTED_VERSION installer for $EXPECTED_ARCH (macOS $EXPECTED_MINIMUM_MACOS+)"
