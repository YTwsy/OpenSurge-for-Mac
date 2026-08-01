#!/usr/bin/env bash

opensurge_is_installed_gui_command() {
  [[ "$#" -eq 2 ]] || return 2
  local command="$1"
  local user_home="$2"
  local control="$user_home/Library/Application Support/OpenSurge/bin/opensurge-control"
  local app="/Applications/OpenSurge.app/Contents/MacOS/OpenSurgeMenuBar"
  local legacy_app="/Applications/OpenSurge Menu Bar.app/Contents/MacOS/OpenSurgeMenuBar"

  case "$command" in
    "$control"|"$control "*|"$app"|"$app "*|"$legacy_app"|"$legacy_app "*)
      return 0
      ;;
  esac
  return 1
}

opensurge_installed_gui_pids() {
  [[ "$#" -eq 2 ]] || return 2
  local uid_value="$1"
  local user_home="$2"
  local process_name pid command

  for process_name in opensurge-control OpenSurgeMenuBar; do
    while IFS= read -r pid; do
      [[ "$pid" =~ ^[0-9]+$ ]] || continue
      command="$(/bin/ps -ww -p "$pid" -o command= 2>/dev/null || true)"
      command="${command#"${command%%[![:space:]]*}"}"
      if opensurge_is_installed_gui_command "$command" "$user_home"; then
        printf '%s\n' "$pid"
      fi
    done < <(/usr/bin/pgrep -u "$uid_value" -x "$process_name" 2>/dev/null || true)
  done
}
