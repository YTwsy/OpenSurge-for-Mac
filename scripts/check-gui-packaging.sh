#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREINSTALL="$ROOT/packaging/pkg-scripts/preinstall"
POSTINSTALL="$ROOT/packaging/pkg-scripts/postinstall"
RECOVERY_STATE="$ROOT/packaging/pkg-scripts/recovery-state.sh"
INSTALLED_PROCESSES="$ROOT/packaging/pkg-scripts/installed-processes.sh"
RELEASE_DEPS="$ROOT/scripts/prepare-gui-release-deps.sh"
MIHOMO_BUILD="$ROOT/scripts/build-opensurge-mihomo.sh"
RELEASE_VERIFY="$ROOT/scripts/verify-unsigned-gui-installer.sh"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release-unsigned.yml"
GUI_COMPONENTS="$ROOT/packaging/gui-components.plist"
APP_ICON_SOURCE="$ROOT/apps/desktop/Resources/OpenSurgeAppIcon.png"
MENU_BAR_ICON_SOURCE="$ROOT/apps/desktop/Resources/OpenSurgeMenuBarIcon.png"
WEB_ICON_SOURCE="$ROOT/web/public/opensurge-icon.png"
WEB_INDEX="$ROOT/web/index.html"
WEB_APP="$ROOT/web/src/App.tsx"
UNINSTALLER="$ROOT/scripts/uninstall-gui.sh"

bash -n "$PREINSTALL" "$POSTINSTALL" "$RECOVERY_STATE" "$INSTALLED_PROCESSES" "$ROOT/scripts/uninstall-gui.sh" \
  "$ROOT/scripts/build-gui-installer.sh" "$ROOT/scripts/build-desktop-app.sh" "$ROOT/scripts/verify-desktop-app.sh" "$RELEASE_DEPS" "$MIHOMO_BUILD" "$RELEASE_VERIFY"
[[ -x "$PREINSTALL" ]] || { echo "preinstall must be executable" >&2; exit 1; }
[[ -x "$RELEASE_DEPS" && -x "$MIHOMO_BUILD" && -x "$RELEASE_VERIFY" ]] || {
  echo "release preparation, patched mihomo build, and verification scripts must be executable" >&2
  exit 1
}

# shellcheck source=packaging/pkg-scripts/recovery-state.sh
source "$RECOVERY_STATE"
for stage in idle complete complete_static; do
  opensurge_recovery_stage_is_terminal "$stage" || {
    echo "terminal recovery stage must allow upgrade: $stage" >&2
    exit 1
  }
done
for stage in "" prepared mac_static router_dhcp_disabled_confirmed gateway_active client_validated client_validation_skipped gateway_stopped_waiting_router_dhcp router_dhcp_restored unknown; do
  if opensurge_recovery_stage_is_terminal "$stage"; then
    echo "incomplete recovery stage must block upgrade: ${stage:-<empty>}" >&2
    exit 1
  fi
done

grep -Fq 'source "$SCRIPT_DIR/recovery-state.sh"' "$PREINSTALL" || {
  echo "preinstall must use the shared recovery terminal-state guard" >&2
  exit 1
}
grep -Fq 'source "$SCRIPT_DIR/installed-processes.sh"' "$PREINSTALL" || {
  echo "preinstall must scope GUI process handling to installed executables" >&2
  exit 1
}
# shellcheck source=packaging/pkg-scripts/installed-processes.sh
source "$INSTALLED_PROCESSES"
opensurge_is_installed_gui_command \
  "/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeMenuBar" "/Users/tester" || {
  echo "current installed menu bar path must be recognized" >&2
  exit 1
}
opensurge_is_installed_gui_command \
  "/Applications/OpenSurge Menu Bar.app/Contents/MacOS/OpenSurgeMenuBar" "/Users/tester" || {
  echo "legacy installed menu bar path must be recognized" >&2
  exit 1
}
opensurge_is_installed_gui_command \
  "/Users/tester/Library/Application Support/OpenSurge/bin/opensurge-control --config /tmp/config.yaml" \
  "/Users/tester" || {
  echo "installed per-user control service path must be recognized" >&2
  exit 1
}
if opensurge_is_installed_gui_command \
  "/Users/tester/project/bin/OpenSurge.app/Contents/MacOS/OpenSurgeMenuBar" "/Users/tester"; then
  echo "a developer menu bar build must not block package installation" >&2
  exit 1
fi
# Both installed generations must stop before the Control Service. Exact paths
# exclude preview builds, developer copies and similarly named executables.
for command in "/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeDesktop" "/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeDesktop --control-dir /tmp/test"; do
  opensurge_is_installed_gui_command "$command" /Users/tester || exit 1
done
for command in "/Applications/OpenSurge Desktop Preview.app/Contents/MacOS/OpenSurgeDesktop" "/Users/tester/project/bin/OpenSurge.app/Contents/MacOS/OpenSurgeDesktop" "/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeDesktopOther" "/Users/other/Library/Application Support/OpenSurge/bin/opensurge-control"; do
  if opensurge_is_installed_gui_command "$command" /Users/tester; then
    echo "installer must not stop an unrelated host: $command" >&2; exit 1
  fi
done
if grep -Eq 'recovery-state|RECOVERY_STAGE|recovery\.json' "$UNINSTALLER"; then
  echo "uninstall must not be blocked by the DHCP recovery state" >&2
  exit 1
fi
grep -Fq 'GATEWAY_STATE=' "$UNINSTALLER" || {
  echo "uninstall must independently read the current gateway state" >&2
  exit 1
}
grep -Fq '[[ "$GATEWAY_STATE" == "stopped" ]]' "$UNINSTALLER" || {
  echo "uninstall must require a stopped gateway" >&2
  exit 1
}
grep -Fq -- '--keep-data|--remove-all' "$UNINSTALLER" || {
  echo "uninstall must support preserving data or removing everything" >&2
  exit 1
}

line_of() {
  local pattern="$1"
  local file="$2"
  awk -v pattern="$pattern" 'index($0, pattern) { print NR; exit }' "$file"
}

recovery_line="$(line_of 'RECOVERY_STAGE=' "$PREINSTALL")"
gui_stop_line="$(line_of 'opensurge_stop_installed_gui_processes "$UID_VALUE" "$USER_HOME"' "$PREINSTALL")"
stop_line="$(line_of '"$RECOVERY_CLI" stop' "$PREINSTALL")"
helper_line="$(line_of 'bootout system/com.opensurge.helper' "$PREINSTALL")"
sleep_release_line="$(line_of 'sleep-prevention-owned' "$PREINSTALL")"

[[ -n "$recovery_line" && -n "$gui_stop_line" && -n "$stop_line" && -n "$sleep_release_line" && -n "$helper_line" ]] || {
  echo "preinstall is missing a required upgrade step" >&2
  exit 1
}
(( recovery_line < gui_stop_line && gui_stop_line < stop_line && stop_line < sleep_release_line && sleep_release_line < helper_line )) || {
  echo "unsafe preinstall order: expected recovery check, GUI/control stop, gateway stop, sleep release, helper bootout" >&2
  exit 1
}

for cleanup in "$PREINSTALL" "$UNINSTALLER"; do
  grep -Fq 'runtime/sleep-prevention-owned' "$cleanup" || {
    echo "sleep prevention cleanup must use the persistent installed-runtime ownership marker: $cleanup" >&2
    exit 1
  }
  grep -Fq '/usr/bin/pmset -a disablesleep 0' "$cleanup" || {
    echo "sleep prevention cleanup must restore system sleep before removing the Helper: $cleanup" >&2
    exit 1
  }
done

grep -Fq 'opensurge_stop_installed_gui_processes "$UID_VALUE" "$USER_HOME"' "$PREINSTALL" || {
  echo "preinstall must stop installed OpenSurge GUI processes" >&2
  exit 1
}
grep -Fq 'RECOVERY_CLI="$SCRIPT_DIR/omg-recovery"' "$PREINSTALL" || {
  echo "preinstall must use the current package recovery CLI" >&2
  exit 1
}
if grep -Eq 'p(kill|grep).*-x (opensurge-control|OpenSurgeMenuBar)' "$PREINSTALL"; then
  echo "preinstall must not block on unrelated same-name developer processes" >&2
  exit 1
fi

# Reproduce the upgrade race that previously made the first install fail: the
# menu bar starts a late bootstrap while it is being terminated, and KeepAlive
# replaces the control PID after the first bootout. The stop helper must bootout
# the exact service again instead of only observing the replacement forever.
(
  test_menu_alive=1
  test_control_alive=1
  test_control_registered=1
  test_late_bootstrap=0
  test_menu_term_count=0
  test_control_term_count=0
  test_control_bootout_count=0

  opensurge_installed_menu_bar_pids() {
    if [[ "$test_menu_alive" -eq 1 ]]; then
      printf '%s\n' 101
    fi
  }
  opensurge_installed_gui_pids() {
    if [[ "$test_control_alive" -eq 1 ]]; then
      printf '%s\n' 201
    fi
    if [[ "$test_menu_alive" -eq 1 ]]; then
      printf '%s\n' 101
    fi
  }
  opensurge_signal_installed_gui_pid() {
    local signal_name="$1"
    local pid="$2"
    if [[ "$signal_name" == "TERM" && "$pid" == "101" ]]; then
      test_menu_term_count=$((test_menu_term_count + 1))
      test_menu_alive=0
      test_late_bootstrap=1
    elif [[ "$signal_name" == "TERM" && "$pid" == "201" ]]; then
      test_control_term_count=$((test_control_term_count + 1))
      if [[ "$test_control_registered" -eq 0 ]]; then
        test_control_alive=0
      fi
    elif [[ "$signal_name" == "KILL" ]]; then
      if [[ "$pid" == "101" ]]; then
        test_menu_alive=0
      elif [[ "$pid" == "201" && "$test_control_registered" -eq 0 ]]; then
        test_control_alive=0
      fi
    fi
  }
  opensurge_bootout_installed_control() {
    test_control_bootout_count=$((test_control_bootout_count + 1))
    if [[ "$test_late_bootstrap" -eq 1 ]]; then
      # Model a bootstrap child completing just after this bootout.
      test_late_bootstrap=0
      test_control_registered=1
      test_control_alive=1
    else
      test_control_registered=0
      test_control_alive=0
    fi
  }
  opensurge_process_wait_tick() { :; }

  opensurge_stop_installed_gui_processes 501 /Users/tester || {
    echo "preinstall process stop must survive a late Control Service bootstrap" >&2
    exit 1
  }
  [[ "$test_menu_term_count" -eq 1 && "$test_control_term_count" -eq 1 ]] || {
    echo "preinstall race regression did not terminate the expected installed processes" >&2
    exit 1
  }
  [[ "$test_control_bootout_count" -eq 2 && "$test_control_alive" -eq 0 ]] || {
    echo "preinstall race regression did not remove the replacement Control Service" >&2
    exit 1
  }
)

# A process at an exact installed path that survives TERM and KILL must still
# fail closed before the gateway/helper upgrade sequence begins.
(
  test_control_bootout_count=0
  opensurge_installed_menu_bar_pids() { printf '%s\n' 301; }
  opensurge_installed_gui_pids() { printf '%s\n' 301; }
  opensurge_signal_installed_gui_pid() { :; }
  opensurge_bootout_installed_control() {
    test_control_bootout_count=$((test_control_bootout_count + 1))
  }
  opensurge_process_wait_tick() { :; }

  if opensurge_stop_installed_gui_processes 501 /Users/tester; then
    echo "preinstall must reject an installed menu bar process that cannot be stopped" >&2
    exit 1
  fi
  [[ "$test_control_bootout_count" -eq 0 ]] || {
    echo "preinstall must stop the menu bar before booting out the Control Service" >&2
    exit 1
  }
)

grep -Fq 'rm -rf "/Applications/OpenSurge Menu Bar.app"' "$POSTINSTALL" || {
  echo "postinstall must remove the legacy menu bar app bundle" >&2
  exit 1
}
grep -Fq '"/Applications/OpenSurge.app" "/Applications/OpenSurge Menu Bar.app"' "$ROOT/scripts/uninstall-gui.sh" || {
  echo "uninstall must remove both current and legacy app bundles" >&2
  exit 1
}
grep -Fq 'install -m 0755 "$ROOT/scripts/uninstall-gui.sh" "$APP_ROOT/share/uninstall-gui.sh"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must install the fixed root-owned uninstall script" >&2
  exit 1
}
grep -Fq 'if [[ ! -f "$ROOT/config.yaml" ]]' "$POSTINSTALL" || {
  echo "postinstall must preserve an existing config during upgrade" >&2
  exit 1
}
grep -Fq 'install -m 0755 "$ROOT/bin/omg" "$PKG_SCRIPTS/omg-recovery"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must stage its current omg as the preinstall recovery CLI" >&2
  exit 1
}
grep -Fq -- '--scripts "$PKG_SCRIPTS"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "pkgbuild must include the staged packaging scripts directory" >&2
  exit 1
}
grep -Fq '"$ROOT/scripts/build-desktop-app.sh" production' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "PKG must build the production Wails host" >&2; exit 1;
}
if grep -Fq 'build-menubar-app.sh' "$ROOT/scripts/build-gui-installer.sh"; then
  echo "PKG must not build the retired Swift host" >&2; exit 1
fi
[[ -s "$APP_ICON_SOURCE" && -s "$MENU_BAR_ICON_SOURCE" && -s "$ROOT/third_party/licenses/wails-MIT.txt" ]]
[[ "$(/usr/libexec/PlistBuddy -c 'Print :0:BundleOverwriteAction' "$GUI_COMPONENTS")" == upgrade ]] || {
  echo "installer must replace the complete App bundle, removing the old executable" >&2; exit 1;
}
[[ "$(/usr/libexec/PlistBuddy -c 'Print :0:BundleIsRelocatable' "$GUI_COMPONENTS")" == false ]]
grep -Fq 'Contents/MacOS/OpenSurgeDesktop' "$ROOT/scripts/build-gui-installer.sh"
grep -Fq '"$PAYLOAD/Applications/OpenSurge.app"' "$ROOT/scripts/build-gui-installer.sh"
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
grep -Fq 'lipo "$executable" -verify_arch "$OPENSURGE_APP_ARCH"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must verify bundled executable architectures" >&2
  exit 1
}
grep -Fq '"$APP_ROOT/bin/opensurge-network" "$APP_ROOT/share/opensurge-control"' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must sign the IPv6 packet broker with the other bundled executables" >&2
  exit 1
}
grep -Fq 'x86_64) GO_ARCH=amd64' "$ROOT/scripts/build-gui-installer.sh" || {
  echo "GUI package must map the Intel Mach-O architecture to Go amd64" >&2
  exit 1
}
grep -Fq 'MIHOMO_SOURCE_ARCHIVE="mihomo-${MIHOMO_VERSION}-source.tar.gz"' "$RELEASE_DEPS" && grep -Fq 'MIHOMO_SOURCE_URL="https://github.com/MetaCubeX/mihomo/archive/' "$RELEASE_DEPS" && grep -Fq 'download_and_verify "$MIHOMO_SOURCE_URL"' "$RELEASE_DEPS" && grep -Fq 'build-opensurge-mihomo.sh' "$RELEASE_DEPS" || {
  echo "release dependencies must build patched mihomo from the pinned source archive" >&2
  exit 1
}
grep -Fq -- '-tags with_gvisor' "$MIHOMO_BUILD" && grep -Fq '0001-opensurge-packet-listener.patch' "$MIHOMO_BUILD" && grep -Fq 'SOURCE_SHA256=971dd453' "$MIHOMO_BUILD" && grep -Fq 'SOURCE_URL=https://github.com/MetaCubeX/mihomo/archive/' "$MIHOMO_BUILD" && grep -Fq 'source_archive_valid' "$MIHOMO_BUILD" || {
  echo "OpenSurge mihomo build must download and verify pinned source before applying the gVisor packet-listener patch" >&2
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
grep -Fq 'arm64' "$RELEASE_WORKFLOW" && grep -Fq 'x86_64' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must build Apple Silicon and Intel packages" >&2
  exit 1
}
grep -Fq 'source_branch="codex/release-v${package_version}"' "$RELEASE_WORKFLOW" || {
  echo "release tags must be built from their versioned release branch" >&2
  exit 1
}
if grep -Fq 'source_branch=master' "$RELEASE_WORKFLOW"; then
  echo "stable releases must not bypass their verified versioned release branch" >&2
  exit 1
fi
grep -Fq 'channel_flag=--prerelease' "$RELEASE_WORKFLOW" || {
  echo "release-candidate tags must publish a GitHub prerelease" >&2
  exit 1
}
grep -Fq -- '--latest' "$RELEASE_WORKFLOW" || {
  echo "stable release workflow must mark the tagged release as latest" >&2
  exit 1
}
grep -Fq 'OPENSURGE_RELEASE_TAG: ${{ steps.version.outputs.release_tag }}' "$RELEASE_WORKFLOW" || {
  echo "release workflow must pass the full tag into the app bundle" >&2
  exit 1
}
grep -Fq '"$OPENSURGE_RELEASE_TAG"' "$RELEASE_WORKFLOW" || {
  echo "release workflow must verify the packaged full release tag" >&2
  exit 1
}
grep -Fq 'OpenSurgeReleaseTag' "$RELEASE_VERIFY" || {
  echo "package verification must inspect the full release tag" >&2
  exit 1
}
grep -Fq 'python3 scripts/release-codename.py "$GITHUB_REF_NAME"' "$RELEASE_WORKFLOW" || {
  echo "release titles must use the shared series codename catalog" >&2
  exit 1
}
[[ "$(python3 "$ROOT/scripts/release-codename.py" v0.2.4)" == "Wind Rose" ]]
[[ "$(python3 "$ROOT/scripts/release-codename.py" v0.3.0-rc.1)" == "Verdilion" ]]
[[ -z "$(python3 "$ROOT/scripts/release-codename.py" v0.4.0)" ]]
grep -Fq -- '--title "$release_title"' "$RELEASE_WORKFLOW" || {
  echo "GitHub release title must include the version-aware series title" >&2
  exit 1
}
grep -Fq 'actions/download-artifact@v8' "$RELEASE_WORKFLOW" || {
  echo "stable release workflow must aggregate both architecture packages" >&2
  exit 1
}
grep -Fq 'verify-unsigned-gui-installer.sh' "$RELEASE_WORKFLOW" || {
  echo "unsigned release workflow must verify the completed package" >&2
  exit 1
}

python3 "$ROOT/tests/packaging/test_installer.py"
echo "GUI packaging checks passed"
