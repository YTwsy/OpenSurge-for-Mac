#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREINSTALL="$ROOT/packaging/pkg-scripts/preinstall"
POSTINSTALL="$ROOT/packaging/pkg-scripts/postinstall"
RECOVERY_STATE="$ROOT/packaging/pkg-scripts/recovery-state.sh"
RELEASE_DEPS="$ROOT/scripts/prepare-gui-release-deps.sh"
RELEASE_VERIFY="$ROOT/scripts/verify-unsigned-gui-installer.sh"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release-unsigned.yml"
MENUBAR_PACKAGE="$ROOT/apps/menubar/Package.swift"
MENUBAR_INFO="$ROOT/apps/menubar/Resources/Info.plist"
APP_ICON_SOURCE="$ROOT/apps/menubar/Resources/OpenSurgeAppIcon.png"
MENU_BAR_ICON_SOURCE="$ROOT/apps/menubar/Resources/OpenSurgeMenuBarIcon.png"
WEB_ICON_SOURCE="$ROOT/web/public/opensurge-icon.png"
WEB_INDEX="$ROOT/web/index.html"
WEB_APP="$ROOT/web/src/App.tsx"

bash -n "$PREINSTALL" "$POSTINSTALL" "$RECOVERY_STATE" "$ROOT/scripts/uninstall-gui.sh" \
  "$ROOT/scripts/build-gui-installer.sh" "$RELEASE_DEPS" "$RELEASE_VERIFY"
[[ -x "$PREINSTALL" ]] || { echo "preinstall must be executable" >&2; exit 1; }
[[ -x "$RELEASE_DEPS" && -x "$RELEASE_VERIFY" ]] || {
  echo "release preparation and verification scripts must be executable" >&2
  exit 1
}

# shellcheck source=packaging/pkg-scripts/recovery-state.sh
source "$RECOVERY_STATE"
for stage in idle complete complete_static; do
  opensurge_recovery_stage_is_terminal "$stage" || {
    echo "terminal recovery stage must allow upgrade and uninstall: $stage" >&2
    exit 1
  }
done
for stage in "" prepared mac_static router_dhcp_disabled_confirmed gateway_active client_validated client_validation_skipped gateway_stopped_waiting_router_dhcp router_dhcp_restored unknown; do
  if opensurge_recovery_stage_is_terminal "$stage"; then
    echo "incomplete recovery stage must block upgrade and uninstall: ${stage:-<empty>}" >&2
    exit 1
  fi
done

grep -Fq 'source "$SCRIPT_DIR/recovery-state.sh"' "$PREINSTALL" || {
  echo "preinstall must use the shared recovery terminal-state guard" >&2
  exit 1
}
grep -Fq 'source "$REPO_ROOT/packaging/pkg-scripts/recovery-state.sh"' "$ROOT/scripts/uninstall-gui.sh" || {
  echo "uninstall must use the shared recovery terminal-state guard" >&2
  exit 1
}

line_of() {
  local pattern="$1"
  local file="$2"
  awk -v pattern="$pattern" 'index($0, pattern) { print NR; exit }' "$file"
}

recovery_line="$(line_of 'RECOVERY_STAGE=' "$PREINSTALL")"
control_line="$(line_of 'bootout "gui/$UID_VALUE/com.opensurge.control"' "$PREINSTALL")"
stop_line="$(line_of '"$ROOT/bin/omg" stop' "$PREINSTALL")"
helper_line="$(line_of 'bootout system/com.opensurge.helper' "$PREINSTALL")"

[[ -n "$recovery_line" && -n "$control_line" && -n "$stop_line" && -n "$helper_line" ]] || {
  echo "preinstall is missing a required upgrade step" >&2
  exit 1
}
(( recovery_line < control_line && control_line < stop_line && stop_line < helper_line )) || {
  echo "unsafe preinstall order: expected recovery check, control bootout, gateway stop, helper bootout" >&2
  exit 1
}

grep -Fq 'pkill -TERM -u "$UID_VALUE" -x OpenSurgeMenuBar' "$PREINSTALL" || {
  echo "preinstall must stop the menu bar app" >&2
  exit 1
}
grep -Fq 'if [[ ! -f "$ROOT/config.yaml" ]]' "$POSTINSTALL" || {
  echo "postinstall must preserve an existing config during upgrade" >&2
  exit 1
}
grep -Fq -- '--scripts "$ROOT/packaging/pkg-scripts"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "pkgbuild must include the packaging scripts directory" >&2
  exit 1
}
grep -Fq 'plutil -replace CFBundleShortVersionString' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must stamp the package version into Info.plist" >&2
  exit 1
}
grep -Fq 'plutil -replace CFBundleVersion' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must stamp the build number into Info.plist" >&2
  exit 1
}
[[ -s "$APP_ICON_SOURCE" && -s "$MENU_BAR_ICON_SOURCE" ]] || {
  echo "menu bar app icon assets must be present" >&2
  exit 1
}
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIconFile' "$MENUBAR_INFO")" == "OpenSurgeAppIcon" ]] || {
  echo "menu bar app Info.plist must reference the OpenSurge app icon" >&2
  exit 1
}
for bundle_name_key in CFBundleName CFBundleDisplayName; do
  [[ "$(/usr/libexec/PlistBuddy -c "Print :$bundle_name_key" "$MENUBAR_INFO")" == "OpenSurge" ]] || {
    echo "menu bar app $bundle_name_key must use the OpenSurge product name" >&2
    exit 1
  }
done
grep -Fq 'OpenSurgeAppIcon.icns' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must generate the app icon resource" >&2
  exit 1
}
grep -Fq 'OpenSurgeMenuBarIcon.png' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must include the monochrome menu bar icon resource" >&2
  exit 1
}
[[ -s "$WEB_ICON_SOURCE" ]] || {
  echo "Web GUI app icon must be present" >&2
  exit 1
}
grep -Fq 'rel="icon" type="image/png" href="/opensurge-icon.png"' "$WEB_INDEX" || {
  echo "Web GUI must expose the OpenSurge browser icon" >&2
  exit 1
}
grep -Fq 'className="brand-mark" src="/opensurge-icon.png"' "$WEB_APP" || {
  echo "Web GUI sidebar must use the OpenSurge app icon" >&2
  exit 1
}
grep -Fq -- '--arch "$ARCH"' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must use the package architecture explicitly" >&2
  exit 1
}
grep -Fq '// swift-tools-version: 5.10' "$MENUBAR_PACKAGE" || {
  echo "menu bar package must remain buildable by the macOS 14 release runner" >&2
  exit 1
}
grep -Fq 'lipo "$executable" -verify_arch "$OPENSURGE_APP_ARCH"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must verify bundled executable architectures" >&2
  exit 1
}
grep -Fq 'actions/attest@v4' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must attest the package provenance" >&2
  exit 1
}
grep -Fq 'actions/upload-artifact@v7' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must use the Node 24 artifact uploader" >&2
  exit 1
}
grep -Fq -- '--prerelease' "$RELEASE_WORKFLOW" || {
  echo "unsigned packages must be published as prereleases" >&2
  exit 1
}
grep -Fq 'verify-unsigned-gui-installer.sh' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must verify the completed package" >&2
  exit 1
}

echo "GUI packaging checks passed"
