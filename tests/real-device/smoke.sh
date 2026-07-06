#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

REAL_DIR="$ROOT/runtime/real-device"
CONFIG_OFF="$REAL_DIR/config-off.yaml"
CONFIG_TUN="$REAL_DIR/config-tun.yaml"
OMG_BIN="$ROOT/bin/omg"

IFACE="${OMG_REAL_DEVICE_IFACE:-en7}"
LAN_IP="${OMG_REAL_DEVICE_LAN_IP:-192.168.50.1}"
RANGE_START="${OMG_REAL_DEVICE_RANGE_START:-192.168.50.100}"
RANGE_END="${OMG_REAL_DEVICE_RANGE_END:-192.168.50.200}"
LEASE_TIME="${OMG_REAL_DEVICE_LEASE_TIME:-30m}"
DOMAIN="${OMG_REAL_DEVICE_DOMAIN:-realtest}"
TEST_HOST="${OMG_REAL_DEVICE_TEST_HOST:-example.com}"
UPSTREAM_IFACE="${OMG_REAL_DEVICE_UPSTREAM_IFACE:-}"
UPSTREAM_PROXY_ENABLED="${OMG_REAL_DEVICE_UPSTREAM_PROXY_ENABLED:-false}"
UPSTREAM_PROXY_NAME="${OMG_REAL_DEVICE_UPSTREAM_PROXY_NAME:-real-device-egress}"
UPSTREAM_PROXY_TYPE="${OMG_REAL_DEVICE_UPSTREAM_PROXY_TYPE:-http}"
UPSTREAM_PROXY_SERVER="${OMG_REAL_DEVICE_UPSTREAM_PROXY_SERVER:-127.0.0.1}"
UPSTREAM_PROXY_PORT="${OMG_REAL_DEVICE_UPSTREAM_PROXY_PORT:-18080}"
UPSTREAM_PROXY_USERNAME="${OMG_REAL_DEVICE_UPSTREAM_PROXY_USERNAME:-}"
UPSTREAM_PROXY_PASSWORD="${OMG_REAL_DEVICE_UPSTREAM_PROXY_PASSWORD:-}"
UPSTREAM_PROXY_MATCH_DOMAIN="${OMG_REAL_DEVICE_UPSTREAM_PROXY_MATCH_DOMAIN:-$TEST_HOST}"

HOST_PATH="${PATH:-/usr/bin:/bin:/usr/sbin:/sbin}"
PATH="$ROOT/runtime/tools/bin:$HOST_PATH:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH

usage() {
  cat <<'EOF'
usage: tests/real-device/smoke.sh <command>

Commands:
  start-off      build, write configs, prompt once for sudo, start explicit-proxy smoke
  start-tun      build, write configs, prompt once for sudo, start TUN smoke
  stop           prompt once for sudo, stop either real-device smoke config
  status         show gateway status and leases without sudo
  doctor         build, write configs, run root doctor
  client-check   show leases and recent logs after a real client joins
  write-configs  write runtime/real-device config-off.yaml and config-tun.yaml

Environment overrides:
  OMG_REAL_DEVICE_IFACE=en7
  OMG_REAL_DEVICE_UPSTREAM_IFACE=en0
  OMG_REAL_DEVICE_LAN_IP=192.168.50.1
  OMG_REAL_DEVICE_RANGE_START=192.168.50.100
  OMG_REAL_DEVICE_RANGE_END=192.168.50.200
  OMG_REAL_DEVICE_UPSTREAM_PROXY_ENABLED=false
  OMG_REAL_DEVICE_UPSTREAM_PROXY_TYPE=http
  OMG_REAL_DEVICE_UPSTREAM_PROXY_SERVER=127.0.0.1
  OMG_REAL_DEVICE_UPSTREAM_PROXY_PORT=18080
  OMG_REAL_DEVICE_UPSTREAM_PROXY_MATCH_DOMAIN=example.com
EOF
}

section() {
  printf '\n== %s ==\n' "$1"
}

require_macos() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "real-device smoke currently targets macOS only" >&2
    exit 1
  fi
}

default_upstream_interface() {
  /sbin/route -n get default | /usr/bin/awk '/interface:/ { print $2; exit }'
}

resolve_upstream_interface() {
  if [[ -n "$UPSTREAM_IFACE" ]]; then
    printf '%s\n' "$UPSTREAM_IFACE"
    return
  fi
  default_upstream_interface
}

write_config() {
  local config=$1 mode=$2 dns_upstream=$3
  local upstream
  upstream="$(resolve_upstream_interface)"
  mkdir -p "$REAL_DIR"
  cat >"$config" <<EOF
gateway:
  interface: "$IFACE"
  lan_ip: "$LAN_IP"
  upstream_interface: "$upstream"

dhcp:
  binary: "./runtime/tools/bin/dnsmasq"
  enabled: true
  range_start: "$RANGE_START"
  range_end: "$RANGE_END"
  lease_time: "$LEASE_TIME"
  domain: "$DOMAIN"

dns:
  listen: "$LAN_IP"
  port: 53
  upstream: "$dns_upstream"

mihomo:
  binary: "./runtime/tools/bin/mihomo"
  config: "./runtime/real-device/mihomo.yaml"
  mixed_port: 17890
  redir_port: 0
  api_addr: "127.0.0.1:19090"
  secret: ""

pf:
  anchor_name: "com.apple/open_mihomo_gateway_real_device"
  redirect_tcp_to: 0

transparent:
  mode: "$mode"
  tun_device: "utun123"
  tun_stack: "mixed"
  tun_auto_route: true
  tun_auto_detect_interface: false
  tun_strict_route: false

upstream_proxy:
  enabled: $UPSTREAM_PROXY_ENABLED
  name: "$UPSTREAM_PROXY_NAME"
  type: "$UPSTREAM_PROXY_TYPE"
  server: "$UPSTREAM_PROXY_SERVER"
  port: $UPSTREAM_PROXY_PORT
  username: "$UPSTREAM_PROXY_USERNAME"
  password: "$UPSTREAM_PROXY_PASSWORD"
  match_domain: "$UPSTREAM_PROXY_MATCH_DOMAIN"

runtime:
  dir: "./runtime/real-device"
EOF
}

write_configs() {
  write_config "$CONFIG_OFF" off ""
  write_config "$CONFIG_TUN" tun "127.0.0.1#1053"
  section "configs"
  printf 'wrote %s\n' "${CONFIG_OFF#$ROOT/}"
  printf 'wrote %s\n' "${CONFIG_TUN#$ROOT/}"
}

build_omg() {
  section "build"
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$OMG_BIN" ./cmd/omg
  "$OMG_BIN" status --config "$CONFIG_OFF" >/dev/null 2>&1 || true
  printf 'built %s\n' "${OMG_BIN#$ROOT/}"
}

run_root() {
  local command=$1
  shift
  local upstream
  upstream="$(resolve_upstream_interface 2>/dev/null || true)"
  if [[ "$EUID" == 0 ]]; then
    "$0" "__root_$command" "$@"
    return
  fi
  sudo env \
    OMG_REAL_DEVICE_IFACE="$IFACE" \
    OMG_REAL_DEVICE_LAN_IP="$LAN_IP" \
    OMG_REAL_DEVICE_RANGE_START="$RANGE_START" \
    OMG_REAL_DEVICE_RANGE_END="$RANGE_END" \
    OMG_REAL_DEVICE_LEASE_TIME="$LEASE_TIME" \
    OMG_REAL_DEVICE_DOMAIN="$DOMAIN" \
    OMG_REAL_DEVICE_TEST_HOST="$TEST_HOST" \
    OMG_REAL_DEVICE_UPSTREAM_IFACE="$upstream" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_ENABLED="$UPSTREAM_PROXY_ENABLED" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_NAME="$UPSTREAM_PROXY_NAME" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_TYPE="$UPSTREAM_PROXY_TYPE" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_SERVER="$UPSTREAM_PROXY_SERVER" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_PORT="$UPSTREAM_PROXY_PORT" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_USERNAME="$UPSTREAM_PROXY_USERNAME" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_PASSWORD="$UPSTREAM_PROXY_PASSWORD" \
    OMG_REAL_DEVICE_UPSTREAM_PROXY_MATCH_DOMAIN="$UPSTREAM_PROXY_MATCH_DOMAIN" \
    /bin/bash "$0" "__root_$command" "$@"
}

stop_one() {
  local config=$1
  "$OMG_BIN" stop --config "$config" || true
}

root_stop() {
  require_macos
  section "stop"
  stop_one "$CONFIG_OFF"
  stop_one "$CONFIG_TUN"

  section "status"
  "$OMG_BIN" status --config "$CONFIG_OFF" || true

  section "host cleanup checks"
  /usr/sbin/sysctl -n net.inet.ip.forwarding || true
  /sbin/pfctl -s Anchors || true
}

root_start() {
  require_macos
  local config=$1 label=$2

  section "stop existing smoke"
  stop_one "$CONFIG_OFF"
  stop_one "$CONFIG_TUN"

  section "bind downstream interface"
  /sbin/ifconfig "$IFACE" inet "$LAN_IP" netmask 255.255.255.0 up
  /sbin/ifconfig "$IFACE"

  section "root doctor"
  env PATH="$PATH" "$OMG_BIN" doctor --config "$config"

  section "start ${label}"
  "$OMG_BIN" start --config "$config"
  sleep 2

  section "status"
  "$OMG_BIN" status --config "$config"

  section "listeners"
  /usr/sbin/lsof -nP -iTCP:17890 -sTCP:LISTEN || true
  /usr/sbin/lsof -nP -iTCP:19090 -sTCP:LISTEN || true
  /usr/sbin/lsof -nP -iUDP:53 || true
  /usr/sbin/lsof -nP -iUDP:1053 || true

  section "mihomo API"
  /usr/bin/curl --fail --silent --show-error --max-time 2 http://127.0.0.1:19090/version || true

  section "DNS"
  /usr/bin/dig "@$LAN_IP" "$TEST_HOST" A +time=2 +tries=1 || true

  section "logs"
  /usr/bin/tail -n 80 "$REAL_DIR/logs/dnsmasq.log" 2>/dev/null || true
  /usr/bin/tail -n 80 "$REAL_DIR/logs/mihomo.log" 2>/dev/null || true

  section "leases"
  "$OMG_BIN" leases --config "$config" || true
}

status_smoke() {
  section "status"
  "$OMG_BIN" status --config "$CONFIG_OFF" || true

  section "leases"
  "$OMG_BIN" leases --config "$CONFIG_OFF" || true
}

client_check() {
  status_smoke

  section "recent dnsmasq log"
  tail -n 120 "$REAL_DIR/logs/dnsmasq.log" 2>/dev/null || true

  section "recent mihomo log"
  tail -n 120 "$REAL_DIR/logs/mihomo.log" 2>/dev/null || true
}

start_off() {
  require_macos
  write_configs
  build_omg
  run_root start "$CONFIG_OFF" explicit-proxy
}

start_tun() {
  require_macos
  write_configs
  build_omg
  run_root start "$CONFIG_TUN" tun
}

doctor_smoke() {
  require_macos
  write_configs
  build_omg
  run_root doctor "$CONFIG_OFF"
}

root_doctor() {
  local config=$1
  require_macos
  /sbin/ifconfig "$IFACE" inet "$LAN_IP" netmask 255.255.255.0 up
  env PATH="$PATH" "$OMG_BIN" doctor --config "$config"
}

case "${1:-}" in
  start-off)
    start_off
    ;;
  start-tun)
    start_tun
    ;;
  stop)
    run_root stop
    ;;
  status)
    status_smoke
    ;;
  doctor)
    doctor_smoke
    ;;
  client-check)
    client_check
    ;;
  write-configs)
    write_configs
    ;;
  __root_start)
    shift
    root_start "$@"
    ;;
  __root_stop)
    shift
    root_stop "$@"
    ;;
  __root_doctor)
    shift
    root_doctor "$@"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
