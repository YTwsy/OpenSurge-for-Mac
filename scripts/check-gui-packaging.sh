#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREINSTALL="$ROOT/packaging/pkg-scripts/preinstall"
POSTINSTALL="$ROOT/packaging/pkg-scripts/postinstall"
RECOVERY_STATE="$ROOT/packaging/pkg-scripts/recovery-state.sh"
RELEASE_DEPS="$ROOT/scripts/prepare-gui-release-deps.sh"
RELEASE_VERIFY="$ROOT/scripts/verify-unsigned-gui-installer.sh"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release-unsigned.yml"

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
grep -Fq -- '--arch "$ARCH"' "$ROOT/scripts/build-menubar-app.sh" || {
  echo "menu bar build must use the package architecture explicitly" >&2
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
grep -Fq -- '--prerelease' "$RELEASE_WORKFLOW" || {
  echo "unsigned packages must be published as prereleases" >&2
  exit 1
}
grep -Fq 'verify-unsigned-gui-installer.sh' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must verify the completed package" >&2
  exit 1
}

echo "GUI packaging checks passed"
