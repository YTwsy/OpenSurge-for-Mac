#!/usr/bin/env bash
set -euo pipefail

MODE="${1:---remove-all}"
case "$MODE" in
  --keep-data|--remove-all) ;;
  *)
    echo "Usage: $0 [--keep-data|--remove-all]" >&2
    exit 64
    ;;
esac

if [[ "$(id -u)" -ne 0 ]]; then exec sudo "$0" "$@"; fi

SYSTEM_ROOT="/Library/Application Support/OpenSurge"
OMG="$SYSTEM_ROOT/bin/omg"
CONFIG="$SYSTEM_ROOT/config.yaml"
CONSOLE_USER="$(stat -f '%Su' /dev/console)"
[[ "$CONSOLE_USER" != "root" && "$CONSOLE_USER" != "loginwindow" ]] || {
  echo "No logged-in GUI user; refusing to guess which user data to remove" >&2
  exit 1
}
USER_HOME="$(dscl . -read "/Users/$CONSOLE_USER" NFSHomeDirectory | awk '{print $2}')"
case "$USER_HOME" in
  ""|"/"|"/Users")
    echo "Unsafe home directory resolved for $CONSOLE_USER" >&2
    exit 1
    ;;
esac
USER_ROOT="$USER_HOME/Library/Application Support/OpenSurge"
UID_VALUE="$(id -u "$CONSOLE_USER")"

[[ -x "$OMG" && -f "$CONFIG" ]] || {
  echo "OpenSurge status components are missing; refusing to uninstall without confirming the gateway is stopped" >&2
  exit 3
}
STATUS_JSON="$("$OMG" status --config "$CONFIG" --format json)"
GATEWAY_STATE="$(printf '%s' "$STATUS_JSON" | /usr/bin/plutil -extract gateway raw -o - - 2>/dev/null || true)"
[[ "$GATEWAY_STATE" == "stopped" ]] || {
  echo "OpenSurge gateway is ${GATEWAY_STATE:-unknown}. Stop the gateway in Network Settings before uninstalling." >&2
  exit 2
}

# The native App waits for this script to return and exits itself afterward.
# Do not terminate OpenSurgeMenuBar here or the authorization result cannot be
# reported to the caller.
launchctl bootout "gui/$UID_VALUE/com.opensurge.control" 2>/dev/null || true
launchctl bootout system/com.opensurge.helper 2>/dev/null || true

rm -f "$USER_HOME/Library/LaunchAgents/com.opensurge.control.plist"
rm -f /Library/LaunchDaemons/com.opensurge.helper.plist
rm -f /Library/PrivilegedHelperTools/com.opensurge.helper
rm -rf "/Applications/OpenSurge.app" "/Applications/OpenSurge Menu Bar.app"
rm -rf /var/run/opensurge

if [[ "$MODE" == "--keep-data" ]]; then
  rm -rf "$USER_ROOT/bin"
  rm -f "$USER_ROOT/control-endpoint.json"
  rm -rf "$SYSTEM_ROOT/bin" "$SYSTEM_ROOT/share"
  RESULT="OpenSurge removed. Configuration, subscriptions, credentials, policy data, runtime records, and logs were preserved for reinstallation."
else
  rm -rf "$USER_ROOT" "$SYSTEM_ROOT" /Library/Logs/OpenSurge
  RESULT="OpenSurge and its configuration, subscriptions, credentials, runtime records, and logs were removed."
fi

/usr/sbin/pkgutil --forget com.opensurge.installer >/dev/null 2>&1 || true
echo "$RESULT"
