#!/usr/bin/env bash

opensurge_recovery_stage_is_terminal() {
  case "${1:-}" in
    idle|complete|complete_static)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}
