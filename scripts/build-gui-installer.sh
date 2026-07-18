#!/usr/bin/env bash
set -euo pipefail
export COPYFILE_DISABLE=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="${OPENSURGE_CONFIG:-$ROOT/examples/config.example.yaml}"
MIHOMO="${OPENSURGE_MIHOMO_BINARY:-$ROOT/bin/mihomo}"
DNSMASQ="${OPENSURGE_DNSMASQ_BINARY:-$(command -v dnsmasq || true)}"
VERSION="${OPENSURGE_VERSION:-0.1.0}"
BUILD_NUMBER="${OPENSURGE_BUILD_NUMBER:-1}"
ARTIFACTS="$ROOT/artifacts/gui-installer"
PAYLOAD="$ARTIFACTS/payload"
APP_ROOT="$PAYLOAD/Library/Application Support/OpenSurge"
LICENSE_ROOT="$APP_ROOT/share/licenses"
GO_BIN="${GO_BIN:-$(command -v go || true)}"
NODE_BIN="${NODE_BIN:-$(command -v node || true)}"
PNPM_BIN="${PNPM_BIN:-$(command -v pnpm || true)}"
export GOCACHE="${GOCACHE:-/private/tmp/opensurge-gui-build-cache}"
export OPENSURGE_VERSION="$VERSION"
export OPENSURGE_BUILD_NUMBER="$BUILD_NUMBER"

[[ -x "$GO_BIN" ]] || { echo "Go toolchain not found; set GO_BIN" >&2; exit 1; }
[[ -x "$NODE_BIN" ]] || { echo "Node.js not found; set NODE_BIN" >&2; exit 1; }
[[ -x "$PNPM_BIN" ]] || { echo "pnpm not found; set PNPM_BIN" >&2; exit 1; }
[[ -f "$CONFIG" ]] || { echo "Config not found: $CONFIG" >&2; exit 1; }
[[ -x "$MIHOMO" ]] || { echo "mihomo binary not found: $MIHOMO" >&2; exit 1; }
[[ -x "$DNSMASQ" ]] || { echo "dnsmasq binary not found: $DNSMASQ" >&2; exit 1; }

cd "$ROOT/web"
export PATH="$(dirname "$NODE_BIN"):$PATH"
"$PNPM_BIN" run test
"$PNPM_BIN" run build
cd "$ROOT"
"$GO_BIN" test ./...
mkdir -p "$ROOT/bin"
"$GO_BIN" build -trimpath -o "$ROOT/bin/omg" ./cmd/omg
"$GO_BIN" build -trimpath -o "$ROOT/bin/opensurge-control" ./cmd/opensurge-control
"$GO_BIN" build -trimpath -o "$ROOT/bin/opensurge-helper" ./cmd/opensurge-helper
"$GO_BIN" build -trimpath -o "$ROOT/bin/opensurge-install-config" ./cmd/opensurge-install-config
"$GO_BIN" build -trimpath -o "$ROOT/bin/opensurge-network" ./cmd/opensurge-network
"$ROOT/bin/opensurge-install-config" --source "$CONFIG" --validate-package-source
if [[ -z "${OPENSURGE_APP_ARCH:-}" ]]; then
  export OPENSURGE_APP_ARCH="$(/usr/bin/lipo -archs "$ROOT/bin/omg")"
fi
"$ROOT/scripts/build-menubar-app.sh"

rm -rf "$ARTIFACTS"
mkdir -p "$APP_ROOT/bin" "$APP_ROOT/share" "$LICENSE_ROOT" "$PAYLOAD/Library/PrivilegedHelperTools" "$PAYLOAD/Applications"
install -m 0755 "$MIHOMO" "$APP_ROOT/bin/mihomo"
install -m 0755 "$DNSMASQ" "$APP_ROOT/bin/dnsmasq"
install -m 0755 "$ROOT/bin/omg" "$APP_ROOT/bin/omg"
install -m 0755 "$ROOT/bin/opensurge-install-config" "$APP_ROOT/bin/opensurge-install-config"
install -m 0755 "$ROOT/bin/opensurge-network" "$APP_ROOT/bin/opensurge-network"
install -m 0755 "$ROOT/bin/opensurge-control" "$APP_ROOT/share/opensurge-control"
install -m 0755 "$ROOT/bin/opensurge-helper" "$PAYLOAD/Library/PrivilegedHelperTools/com.opensurge.helper"
install -m 0644 "$CONFIG" "$APP_ROOT/share/config.yaml"
install -m 0644 "$ROOT/packaging/launchd/com.opensurge.control.plist" "$APP_ROOT/share/com.opensurge.control.plist"
install -m 0644 "$ROOT/packaging/launchd/com.opensurge.helper.plist" "$APP_ROOT/share/com.opensurge.helper.plist"
install -m 0644 "$ROOT/LICENSE" "$LICENSE_ROOT/GPL-3.0.txt"
install -m 0644 "$ROOT/third_party/licenses/dnsmasq-COPYING" "$LICENSE_ROOT/GPL-2.0.txt"
install -m 0644 "$ROOT/third_party/licenses/Apache-2.0.txt" "$LICENSE_ROOT/Apache-2.0.txt"
install -m 0644 "$ROOT/third_party/licenses/yaml-v3-LICENSE" "$LICENSE_ROOT/yaml-v3-LICENSE"
install -m 0644 "$ROOT/third_party/licenses/react-MIT.txt" "$LICENSE_ROOT/react-MIT.txt"
install -m 0644 "$ROOT/THIRD_PARTY_NOTICES.md" "$LICENSE_ROOT/THIRD_PARTY_NOTICES.md"
ditto --norsrc --noextattr "$ROOT/bin/OpenSurge Menu Bar.app" "$PAYLOAD/Applications/OpenSurge Menu Bar.app"
xattr -cr "$PAYLOAD"

for executable in \
  "$PAYLOAD/Applications/OpenSurge Menu Bar.app/Contents/MacOS/OpenSurgeMenuBar" \
  "$APP_ROOT/bin/omg" \
  "$APP_ROOT/bin/opensurge-install-config" \
  "$APP_ROOT/bin/opensurge-network" \
  "$APP_ROOT/share/opensurge-control" \
  "$PAYLOAD/Library/PrivilegedHelperTools/com.opensurge.helper" \
  "$APP_ROOT/bin/mihomo" \
  "$APP_ROOT/bin/dnsmasq"; do
  /usr/bin/lipo "$executable" -verify_arch "$OPENSURGE_APP_ARCH" || {
    echo "packaged executable does not support $OPENSURGE_APP_ARCH: $executable" >&2
    exit 1
  }
done

if [[ -n "${OPENSURGE_CODESIGN_IDENTITY:-}" ]]; then
  codesign --force --options runtime --timestamp --sign "$OPENSURGE_CODESIGN_IDENTITY" "$PAYLOAD/Library/PrivilegedHelperTools/com.opensurge.helper"
  codesign --force --options runtime --timestamp --sign "$OPENSURGE_CODESIGN_IDENTITY" "$APP_ROOT/bin/omg" "$APP_ROOT/bin/opensurge-install-config" "$APP_ROOT/share/opensurge-control"
  codesign --force --deep --options runtime --timestamp --sign "$OPENSURGE_CODESIGN_IDENTITY" "$PAYLOAD/Applications/OpenSurge Menu Bar.app"
fi

PKG_ARGS=(
  --root "$PAYLOAD"
  --component-plist "$ROOT/packaging/gui-components.plist"
  --scripts "$ROOT/packaging/pkg-scripts"
  --identifier com.opensurge.installer
  --version "$VERSION"
  --install-location /
)
if [[ -n "${OPENSURGE_INSTALLER_IDENTITY:-}" ]]; then PKG_ARGS+=(--sign "$OPENSURGE_INSTALLER_IDENTITY"); fi
pkgbuild "${PKG_ARGS[@]}" "$ARTIFACTS/OpenSurge-for-Mac-$VERSION.pkg"
echo "$ARTIFACTS/OpenSurge-for-Mac-$VERSION.pkg"
