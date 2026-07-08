#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

SAME_DIR="$ROOT/runtime/same-lan"
CONFIG_TUN="$SAME_DIR/config-tun.yaml"
OMG_BIN="$ROOT/bin/omg"

IFACE="${OMG_SAME_LAN_IFACE:-}"
MAC_IP="${OMG_SAME_LAN_MAC_IP:-}"
TEST_HOST="${OMG_SAME_LAN_TEST_HOST:-example.com}"
ADB_BIN="${ADB:-adb}"
ADB_SERIAL="${OMG_SAME_LAN_ADB_SERIAL:-}"

UPSTREAM_PROXY_ENABLED="${OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED:-false}"
UPSTREAM_PROXY_NAME="${OMG_SAME_LAN_UPSTREAM_PROXY_NAME:-same-lan-egress}"
UPSTREAM_PROXY_TYPE="${OMG_SAME_LAN_UPSTREAM_PROXY_TYPE:-http}"
UPSTREAM_PROXY_SERVER="${OMG_SAME_LAN_UPSTREAM_PROXY_SERVER:-127.0.0.1}"
UPSTREAM_PROXY_PORT="${OMG_SAME_LAN_UPSTREAM_PROXY_PORT:-18080}"
UPSTREAM_PROXY_USERNAME="${OMG_SAME_LAN_UPSTREAM_PROXY_USERNAME:-}"
UPSTREAM_PROXY_PASSWORD="${OMG_SAME_LAN_UPSTREAM_PROXY_PASSWORD:-}"
UPSTREAM_PROXY_MATCH_DOMAIN="${OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN:-$TEST_HOST}"

HOST_PATH="${PATH:-/usr/bin:/bin:/usr/sbin:/sbin}"
PATH="$ROOT/runtime/tools/bin:$HOST_PATH:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH

usage() {
  cat <<'EOF'
usage: tests/same-lan/smoke.sh <command>

Commands:
  start-tun     build, write config, prompt once for sudo, start same-LAN TUN smoke
  stop          prompt once for sudo, stop same-LAN smoke config
  status        show gateway status and recent logs without sudo
  adb-check     verify one Android client over ADB after its gateway/DNS point at the Mac
  write-config  write runtime/same-lan/config-tun.yaml

Environment overrides:
  OMG_SAME_LAN_IFACE=en0
  OMG_SAME_LAN_MAC_IP=192.168.1.20
  OMG_SAME_LAN_TEST_HOST=example.com
  OMG_SAME_LAN_ADB_SERIAL=<adb-serial>
  OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED=false
  OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=http
  OMG_SAME_LAN_UPSTREAM_PROXY_SERVER=127.0.0.1
  OMG_SAME_LAN_UPSTREAM_PROXY_PORT=18080
  OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN=example.com
EOF
}

section() {
  printf '\n== %s ==\n' "$1"
}

require_macos() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "same-LAN smoke currently targets macOS only" >&2
    exit 1
  fi
}

default_interface() {
  /sbin/route -n get default | /usr/bin/awk '/interface:/ { print $2; exit }'
}

resolve_interface() {
  if [[ -n "$IFACE" ]]; then
    printf '%s\n' "$IFACE"
    return
  fi
  default_interface
}

interface_ipv4() {
  local iface=$1
  /usr/sbin/ipconfig getifaddr "$iface" 2>/dev/null || \
    /sbin/ifconfig "$iface" | /usr/bin/awk '/inet / && $2 != "127.0.0.1" { print $2; exit }'
}

resolve_mac_ip() {
  local iface=$1
  if [[ -n "$MAC_IP" ]]; then
    printf '%s\n' "$MAC_IP"
    return
  fi
  interface_ipv4 "$iface"
}

write_config() {
  local iface mac_ip
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  if [[ -z "$iface" || -z "$mac_ip" ]]; then
    echo "could not resolve same-LAN interface or Mac IPv4 address" >&2
    exit 1
  fi

  mkdir -p "$SAME_DIR"
  cat >"$CONFIG_TUN" <<EOF
gateway:
  mode: "same_lan"
  interface: "$iface"
  lan_ip: "$mac_ip"
  upstream_interface: "$iface"

dhcp:
  binary: "./runtime/tools/bin/dnsmasq"
  enabled: false
  range_start: "192.168.50.100"
  range_end: "192.168.50.200"
  lease_time: "30m"
  domain: "same-lan"

dns:
  listen: "$mac_ip"
  port: 53
  upstream: "127.0.0.1#1053"

mihomo:
  binary: "./runtime/tools/bin/mihomo"
  config: "./runtime/same-lan/mihomo.yaml"
  profile_mode: "managed"
  profile: ""
  mixed_port: 17890
  redir_port: 0
  api_addr: "127.0.0.1:19090"
  secret: ""

pf:
  anchor_name: "com.apple/open_mihomo_gateway_same_lan"
  redirect_tcp_to: 0

transparent:
  mode: "tun"
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
  dir: "./runtime/same-lan"
EOF

  section "config"
  printf 'wrote %s\n' "${CONFIG_TUN#$ROOT/}"
  printf 'same-LAN interface=%s mac_ip=%s test_host=%s\n' "$iface" "$mac_ip" "$TEST_HOST"
}

build_omg() {
  section "build"
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$OMG_BIN" ./cmd/omg
  "$OMG_BIN" status --config "$CONFIG_TUN" >/dev/null 2>&1 || true
  printf 'built %s\n' "${OMG_BIN#$ROOT/}"
}

run_root() {
  local command=$1
  shift
  local iface mac_ip
  iface="$(resolve_interface 2>/dev/null || true)"
  mac_ip="$(resolve_mac_ip "$iface" 2>/dev/null || true)"
  if [[ "$EUID" == 0 ]]; then
    "$0" "__root_$command" "$@"
    return
  fi
  sudo env \
    OMG_SAME_LAN_IFACE="$iface" \
    OMG_SAME_LAN_MAC_IP="$mac_ip" \
    OMG_SAME_LAN_TEST_HOST="$TEST_HOST" \
    OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED="$UPSTREAM_PROXY_ENABLED" \
    OMG_SAME_LAN_UPSTREAM_PROXY_NAME="$UPSTREAM_PROXY_NAME" \
    OMG_SAME_LAN_UPSTREAM_PROXY_TYPE="$UPSTREAM_PROXY_TYPE" \
    OMG_SAME_LAN_UPSTREAM_PROXY_SERVER="$UPSTREAM_PROXY_SERVER" \
    OMG_SAME_LAN_UPSTREAM_PROXY_PORT="$UPSTREAM_PROXY_PORT" \
    OMG_SAME_LAN_UPSTREAM_PROXY_USERNAME="$UPSTREAM_PROXY_USERNAME" \
    OMG_SAME_LAN_UPSTREAM_PROXY_PASSWORD="$UPSTREAM_PROXY_PASSWORD" \
    OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN="$UPSTREAM_PROXY_MATCH_DOMAIN" \
    /bin/bash "$0" "__root_$command" "$@"
}

root_stop() {
  require_macos
  section "stop"
  "$OMG_BIN" stop --config "$CONFIG_TUN" || true

  section "status"
  "$OMG_BIN" status --config "$CONFIG_TUN" || true

  section "host cleanup checks"
  /usr/sbin/sysctl -n net.inet.ip.forwarding || true
  /sbin/pfctl -s Anchors || true
}

root_start() {
  require_macos

  section "stop existing same-LAN smoke"
  "$OMG_BIN" stop --config "$CONFIG_TUN" || true

  section "root doctor"
  env PATH="$PATH" "$OMG_BIN" doctor --config "$CONFIG_TUN"

  section "start same-LAN TUN"
  "$OMG_BIN" start --config "$CONFIG_TUN"
  sleep 2

  section "status"
  "$OMG_BIN" status --config "$CONFIG_TUN"

  section "listeners"
  /usr/sbin/lsof -nP -iTCP:17890 -sTCP:LISTEN || true
  /usr/sbin/lsof -nP -iTCP:19090 -sTCP:LISTEN || true
  /usr/sbin/lsof -nP -iUDP:53 || true
  /usr/sbin/lsof -nP -iUDP:1053 || true

  section "mihomo API"
  /usr/bin/curl --fail --silent --show-error --max-time 2 http://127.0.0.1:19090/version || true

  section "DNS"
  local mac_ip
  mac_ip="$(resolve_mac_ip "$(resolve_interface)")"
  /usr/bin/dig "@$mac_ip" "$TEST_HOST" A +time=2 +tries=1 || true

  section "logs"
  /usr/bin/tail -n 80 "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null || true
  /usr/bin/tail -n 80 "$SAME_DIR/logs/mihomo.log" 2>/dev/null || true
}

status_smoke() {
  section "status"
  "$OMG_BIN" status --config "$CONFIG_TUN" || true

  section "recent dnsmasq log"
  /usr/bin/tail -n 120 "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null || true

  section "recent mihomo log"
  /usr/bin/tail -n 120 "$SAME_DIR/logs/mihomo.log" 2>/dev/null || true
}

select_adb_device() {
  if [[ -n "$ADB_SERIAL" ]]; then
    return
  fi
  local devices count
  devices="$("$ADB_BIN" devices | /usr/bin/awk 'NR > 1 && $2 == "device" { print $1 }')"
  count="$(printf '%s\n' "$devices" | /usr/bin/sed '/^$/d' | /usr/bin/wc -l | /usr/bin/tr -d ' ')"
  case "$count" in
    0)
      echo "no authorized ADB device found" >&2
      "$ADB_BIN" devices >&2 || true
      exit 1
      ;;
    1)
      ADB_SERIAL="$devices"
      ;;
    *)
      echo "multiple ADB devices found; set OMG_SAME_LAN_ADB_SERIAL" >&2
      "$ADB_BIN" devices >&2 || true
      exit 1
      ;;
  esac
}

adb_cmd() {
  if [[ -n "$ADB_SERIAL" ]]; then
    "$ADB_BIN" -s "$ADB_SERIAL" "$@"
  else
    "$ADB_BIN" "$@"
  fi
}

adb_shell() {
  adb_cmd shell "$@"
}

adb_check() {
  local iface mac_ip
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  select_adb_device

  section "adb device"
  adb_cmd shell 'printf "model=%s\nserial=%s\n" "$(getprop ro.product.model)" "$(getprop ro.serialno)"'

  section "adb route"
  adb_shell 'ip -4 addr show wlan0 || true'
  local android_ip
  android_ip="$(adb_shell 'ip -4 addr show wlan0 2>/dev/null' | /usr/bin/awk '/inet / { sub(/\/.*/, "", $2); print $2; exit }' | /usr/bin/tr -d '\r')"
  local route
  route="$(adb_shell 'ip -4 route get 1.1.1.1 2>/dev/null || ip route get 1.1.1.1 2>/dev/null || ip -4 route show default 2>/dev/null || ip route show default 2>/dev/null' | tr -d '\r')"
  printf '%s\n' "$route"
  if [[ "$route" != *"via $mac_ip"* ]]; then
    echo "Android effective route is not via Mac $mac_ip; set this test phone gateway to $mac_ip before rerunning adb-check" >&2
    exit 1
  fi

  section "adb dns"
  set +e
  adb_shell "if command -v nslookup >/dev/null 2>&1; then nslookup $TEST_HOST $mac_ip; elif command -v dig >/dev/null 2>&1; then dig @$mac_ip $TEST_HOST A; else exit 127; fi"
  local dns_status=$?
  set -e
  case "$dns_status" in
    0)
      ;;
    127)
      echo "no nslookup or dig on Android device; DNS will be inferred from the TCP probe and Mac dnsmasq log"
      ;;
    *)
      echo "Android DNS probe failed with status $dns_status" >&2
      exit "$dns_status"
      ;;
  esac

  section "adb no explicit proxy"
  adb_shell 'settings get global http_proxy || true'

  section "adb https probe"
  adb_shell "if command -v curl >/dev/null 2>&1; then curl -I -L --max-time 10 https://$TEST_HOST/; elif command -v wget >/dev/null 2>&1; then wget -S -O - https://$TEST_HOST/; elif command -v nc >/dev/null 2>&1; then nc -z -w 10 $TEST_HOST 443; else echo 'no curl, wget, or nc on Android device'; exit 21; fi"

  section "host logs"
  if [[ -n "$android_ip" ]]; then
    printf 'expect Android source IP in dnsmasq log: %s\n' "$android_ip"
  fi
  /usr/bin/tail -n 120 "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null || true
  /usr/bin/tail -n 120 "$SAME_DIR/logs/mihomo.log" 2>/dev/null || true
}

start_tun() {
  require_macos
  write_config
  build_omg
  run_root start
}

stop_smoke() {
  require_macos
  write_config
  build_omg
  run_root stop
}

case "${1:-}" in
  start-tun)
    start_tun
    ;;
  stop)
    stop_smoke
    ;;
  status)
    status_smoke
    ;;
  adb-check)
    adb_check
    ;;
  write-config)
    write_config
    ;;
  __root_start)
    shift
    root_start "$@"
    ;;
  __root_stop)
    shift
    root_stop "$@"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
