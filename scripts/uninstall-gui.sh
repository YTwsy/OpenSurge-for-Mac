#!/usr/bin/env bash
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then exec sudo "$0" "$@"; fi
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=packaging/pkg-scripts/recovery-state.sh
source "$REPO_ROOT/packaging/pkg-scripts/recovery-state.sh"
CONSOLE_USER="$(stat -f '%Su' /dev/console)"
[[ "$CONSOLE_USER" != "root" && "$CONSOLE_USER" != "loginwindow" ]] || { echo "No logged-in GUI user; refusing to guess which user data to remove" >&2; exit 1; }
USER_HOME="$(dscl . -read "/Users/$CONSOLE_USER" NFSHomeDirectory | awk '{print $2}')"
RECOVERY="$USER_HOME/Library/Application Support/OpenSurge/recovery.json"
if [[ -f "$RECOVERY" ]]; then
  RECOVERY_STAGE="$(/usr/bin/plutil -extract stage raw -o - "$RECOVERY" 2>/dev/null || true)"
  if ! opensurge_recovery_stage_is_terminal "$RECOVERY_STAGE"; then
    echo "OpenSurge recovery is incomplete. Finish same-LAN DHCP recovery before uninstalling: $RECOVERY" >&2
    exit 2
  fi
fi
UID_VALUE="$(id -u "$CONSOLE_USER")"
launchctl bootout "gui/$UID_VALUE/com.opensurge.control" 2>/dev/null || true
if [[ -x "/Library/Application Support/OpenSurge/bin/omg" && -f "/Library/Application Support/OpenSurge/config.yaml" ]]; then
  "/Library/Application Support/OpenSurge/bin/omg" stop --config "/Library/Application Support/OpenSurge/config.yaml"
fi
launchctl bootout system/com.opensurge.helper 2>/dev/null || true
rm -f "$USER_HOME/Library/LaunchAgents/com.opensurge.control.plist"
rm -rf "$USER_HOME/Library/Application Support/OpenSurge" "/Applications/OpenSurge Menu Bar.app"
rm -f /Library/LaunchDaemons/com.opensurge.helper.plist /Library/PrivilegedHelperTools/com.opensurge.helper
rm -rf "/Library/Application Support/OpenSurge" /Library/Logs/OpenSurge /var/run/opensurge
echo "OpenSurge GUI components removed. Network recovery state was complete."
