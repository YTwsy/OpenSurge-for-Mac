#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREINSTALL="$ROOT/packaging/pkg-scripts/preinstall"
POSTINSTALL="$ROOT/packaging/pkg-scripts/postinstall"

bash -n "$PREINSTALL" "$POSTINSTALL" "$ROOT/scripts/uninstall-gui.sh" "$ROOT/scripts/build-gui-installer.sh"
[[ -x "$PREINSTALL" ]] || { echo "preinstall must be executable" >&2; exit 1; }

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

echo "GUI packaging checks passed"
