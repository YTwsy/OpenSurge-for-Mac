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

opensurge_installed_named_gui_pids() {
  [[ "$#" -ge 3 ]] || return 2
  local uid_value="$1"
  local user_home="$2"
  local process_name pid command
  shift 2

  for process_name in "$@"; do
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

opensurge_installed_menu_bar_pids() {
  [[ "$#" -eq 2 ]] || return 2
  opensurge_installed_named_gui_pids "$1" "$2" OpenSurgeMenuBar
}

opensurge_installed_gui_pids() {
  [[ "$#" -eq 2 ]] || return 2
  opensurge_installed_named_gui_pids "$1" "$2" opensurge-control OpenSurgeMenuBar
}

opensurge_signal_installed_gui_pid() {
  [[ "$#" -eq 2 ]] || return 2
  local signal_name="$1"
  local pid="$2"

  case "$signal_name" in
    TERM|KILL) ;;
    *) return 2 ;;
  esac
  [[ "$pid" =~ ^[0-9]+$ ]] || return 2
  /bin/kill "-$signal_name" "$pid" 2>/dev/null || true
}

opensurge_bootout_installed_control() {
  [[ "$#" -eq 1 ]] || return 2
  /bin/launchctl bootout "gui/$1/com.opensurge.control" 2>/dev/null || true
}

opensurge_process_wait_tick() {
  sleep 0.1
}

# The menu bar can bootstrap com.opensurge.control when polling observes that
# the service is unavailable. Stop it first, then keep booting out the exact
# launchd service while rescanning installed executable paths. This also closes
# the smaller race where a launchctl child finishes bootstrap after the menu bar
# process itself has already exited.
opensurge_stop_installed_gui_processes() {
  [[ "$#" -eq 2 ]] || return 2
  local uid_value="$1"
  local user_home="$2"
  local attempt pid menu_pids gui_pids

  menu_pids="$(opensurge_installed_menu_bar_pids "$uid_value" "$user_home")"
  for pid in $menu_pids; do
    opensurge_signal_installed_gui_pid TERM "$pid"
  done

  for attempt in {1..50}; do
    menu_pids="$(opensurge_installed_menu_bar_pids "$uid_value" "$user_home")"
    [[ -z "$menu_pids" ]] && break
    opensurge_process_wait_tick
  done
  menu_pids="$(opensurge_installed_menu_bar_pids "$uid_value" "$user_home")"
  if [[ -n "$menu_pids" ]]; then
    for pid in $menu_pids; do
      opensurge_signal_installed_gui_pid KILL "$pid"
    done
    for attempt in {1..20}; do
      menu_pids="$(opensurge_installed_menu_bar_pids "$uid_value" "$user_home")"
      [[ -z "$menu_pids" ]] && break
      opensurge_process_wait_tick
    done
  fi
  menu_pids="$(opensurge_installed_menu_bar_pids "$uid_value" "$user_home")"
  [[ -z "$menu_pids" ]] || return 1

  for attempt in {1..50}; do
    opensurge_bootout_installed_control "$uid_value"
    gui_pids="$(opensurge_installed_gui_pids "$uid_value" "$user_home")"
    [[ -z "$gui_pids" ]] && return 0
    for pid in $gui_pids; do
      opensurge_signal_installed_gui_pid TERM "$pid"
    done
    opensurge_process_wait_tick
  done

  for attempt in {1..20}; do
    opensurge_bootout_installed_control "$uid_value"
    gui_pids="$(opensurge_installed_gui_pids "$uid_value" "$user_home")"
    [[ -z "$gui_pids" ]] && return 0
    for pid in $gui_pids; do
      opensurge_signal_installed_gui_pid KILL "$pid"
    done
    opensurge_process_wait_tick
  done

  opensurge_bootout_installed_control "$uid_value"
  gui_pids="$(opensurge_installed_gui_pids "$uid_value" "$user_home")"
  [[ -z "$gui_pids" ]]
}
