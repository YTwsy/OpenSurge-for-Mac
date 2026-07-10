#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

SAME_DIR="$ROOT/runtime/same-lan"
CONFIG_TUN="$SAME_DIR/config-tun.yaml"
OMG_BIN="$ROOT/bin/omg"
EGRESS_PROBE_BIN="$SAME_DIR/egress-probe"
EGRESS_PROBE_PID_FILE="$SAME_DIR/egress-probe.pid"
EGRESS_PROVIDER="$SAME_DIR/tun-egress-provider.yaml"
EGRESS_PROFILE_TEMPLATE="$ROOT/tests/lab/mihomo-profile.imported-tun-egress.yaml"
EGRESS_PROFILE="$SAME_DIR/mihomo-profile.imported-tun-egress.yaml"
EGRESS_ORIGIN_PORT="${OMG_SAME_LAN_TUN_EGRESS_ORIGIN_PORT:-19093}"
EGRESS_PROXY_PORT="${OMG_SAME_LAN_TUN_EGRESS_PROXY_PORT:-19094}"
EGRESS_PROVIDER_URL="http://127.0.0.1:$EGRESS_ORIGIN_PORT/tun-egress-provider.yaml"

IFACE="${OMG_SAME_LAN_IFACE:-}"
MAC_IP="${OMG_SAME_LAN_MAC_IP:-}"
TEST_HOST="${OMG_SAME_LAN_TEST_HOST:-example.com}"
ADB_BIN="${ADB:-adb}"
ADB_SERIAL="${OMG_SAME_LAN_ADB_SERIAL:-}"
IMPORTED_EGRESS_ENABLED="${OMG_SAME_LAN_IMPORTED_EGRESS:-false}"

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
  start-tun                    build, write config, prompt once for sudo, start same-LAN TUN smoke
  start-tun-imported-egress    start same-LAN TUN with imported provider-backed egress switching fixture
  stop                         prompt once for sudo, stop same-LAN smoke config
  status                       show gateway status and recent logs without sudo
  adb-check                    verify one Android client over ADB after its gateway/DNS point at the Mac
  adb-check-imported-egress    verify policy-select switches same-LAN TUN egress on one Android client
  write-config                 write runtime/same-lan/config-tun.yaml

Environment overrides:
  OMG_SAME_LAN_IFACE=en0
  OMG_SAME_LAN_MAC_IP=192.168.1.20
  OMG_SAME_LAN_TEST_HOST=example.com
  OMG_SAME_LAN_ADB_SERIAL=<adb-serial>
  OMG_SAME_LAN_IMPORTED_EGRESS=false
  OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED=false
  OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=http
  OMG_SAME_LAN_UPSTREAM_PROXY_SERVER=127.0.0.1
  OMG_SAME_LAN_UPSTREAM_PROXY_PORT=18080
  OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN=example.com
  OMG_SAME_LAN_TUN_EGRESS_ORIGIN_PORT=19093
  OMG_SAME_LAN_TUN_EGRESS_PROXY_PORT=19094
EOF
}

section() {
  printf '\n== %s ==\n' "$1"
}

flag_enabled() {
  case "$1" in
    1|true|TRUE|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

imported_egress_enabled() {
  flag_enabled "$IMPORTED_EGRESS_ENABLED"
}

sed_escape() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
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

write_tun_egress_provider() {
  cat >"$EGRESS_PROVIDER" <<EOF
proxies:
  - name: "egress-proxy"
    type: http
    server: "127.0.0.1"
    port: $EGRESS_PROXY_PORT
EOF
}

render_tun_egress_profile() {
  [[ -f "$EGRESS_PROFILE_TEMPLATE" ]] || { echo "missing imported egress profile template: $EGRESS_PROFILE_TEMPLATE" >&2; exit 1; }
  mkdir -p "$SAME_DIR"
  write_tun_egress_provider
  sed \
    -e "s|__TUN_EGRESS_PROVIDER_URL__|$(sed_escape "$EGRESS_PROVIDER_URL")|g" \
    -e "s|__TUN_EGRESS_HOST__|$(sed_escape "$TEST_HOST")|g" \
    "$EGRESS_PROFILE_TEMPLATE" >"$EGRESS_PROFILE"
}

write_config() {
  local iface mac_ip profile_mode profile_path upstream_proxy_enabled
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  if [[ -z "$iface" || -z "$mac_ip" ]]; then
    echo "could not resolve same-LAN interface or Mac IPv4 address" >&2
    exit 1
  fi

  mkdir -p "$SAME_DIR"
  profile_mode="managed"
  profile_path=""
  upstream_proxy_enabled="$UPSTREAM_PROXY_ENABLED"
  if imported_egress_enabled; then
    if flag_enabled "$UPSTREAM_PROXY_ENABLED"; then
      echo "same-LAN imported egress uses its own controlled provider/proxy; leave OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED unset" >&2
      exit 1
    fi
    render_tun_egress_profile
    profile_mode="imported"
    profile_path="./runtime/same-lan/$(basename "$EGRESS_PROFILE")"
    upstream_proxy_enabled=false
  fi

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
  profile_mode: "$profile_mode"
  profile: "$profile_path"
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
  enabled: $upstream_proxy_enabled
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
  if imported_egress_enabled; then
    printf 'imported egress profile=%s provider=%s proxy=127.0.0.1:%s\n' \
      "${EGRESS_PROFILE#$ROOT/}" "$EGRESS_PROVIDER_URL" "$EGRESS_PROXY_PORT"
  fi
}

build_omg() {
  section "build"
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$OMG_BIN" ./cmd/omg
  "$OMG_BIN" status --config "$CONFIG_TUN" >/dev/null 2>&1 || true
  printf 'built %s\n' "${OMG_BIN#$ROOT/}"
}

build_egress_probe() {
  section "build egress probe"
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$EGRESS_PROBE_BIN" ./tests/integration/egressprobe
  printf 'built %s\n' "${EGRESS_PROBE_BIN#$ROOT/}"
}

stop_egress_probe() {
  local pid
  if [[ -r "$EGRESS_PROBE_PID_FILE" ]]; then
    pid="$(cat "$EGRESS_PROBE_PID_FILE" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 20); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
    fi
    rm -f "$EGRESS_PROBE_PID_FILE"
  fi
}

start_egress_probe() {
  local log_file pid
  section "start egress probe"
  stop_egress_probe
  mkdir -p "$SAME_DIR/logs"
  rm -rf "$SAME_DIR/egress"
  log_file="$SAME_DIR/logs/egress-probe.log"
  "$EGRESS_PROBE_BIN" \
    --origin "127.0.0.1:$EGRESS_ORIGIN_PORT" \
    --proxy "127.0.0.1:$EGRESS_PROXY_PORT" \
    --provider-file "$EGRESS_PROVIDER" \
    --provider-path "/tun-egress-provider.yaml" \
    --log-dir "$SAME_DIR/egress" >"$log_file" 2>&1 &
  pid=$!
  printf '%s\n' "$pid" >"$EGRESS_PROBE_PID_FILE"
  for _ in $(seq 1 50); do
    if grep -Fq READY "$log_file" 2>/dev/null; then
      printf 'TUN egress probe ready: provider=%s proxy=127.0.0.1:%s\n' "$EGRESS_PROVIDER_URL" "$EGRESS_PROXY_PORT"
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "TUN egress probe exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      rm -f "$EGRESS_PROBE_PID_FILE"
      exit 1
    fi
    sleep 0.1
  done
  echo "TUN egress probe did not become ready" >&2
  cat "$log_file" >&2 || true
  stop_egress_probe
  exit 1
}

require_egress_probe_running() {
  local pid
  if [[ ! -r "$EGRESS_PROBE_PID_FILE" ]]; then
    echo "same-LAN imported egress probe is not running; start with make same-lan-start-tun-imported-egress" >&2
    exit 1
  fi
  pid="$(cat "$EGRESS_PROBE_PID_FILE")"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    echo "same-LAN imported egress probe pid is stale; restart with make same-lan-start-tun-imported-egress" >&2
    exit 1
  fi
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
    OMG_SAME_LAN_IMPORTED_EGRESS="$IMPORTED_EGRESS_ENABLED" \
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

  if [[ -r "$EGRESS_PROBE_PID_FILE" ]]; then
    section "same-LAN egress probe"
    printf 'pid=%s provider=%s proxy=127.0.0.1:%s\n' "$(cat "$EGRESS_PROBE_PID_FILE")" "$EGRESS_PROVIDER_URL" "$EGRESS_PROXY_PORT"
    /usr/bin/tail -n 80 "$SAME_DIR/logs/egress-probe.log" 2>/dev/null || true
    /usr/bin/tail -n 80 "$SAME_DIR/egress/proxy.log" 2>/dev/null || true
  fi
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

adb_https_probe() {
  adb_shell "if command -v curl >/dev/null 2>&1; then curl -I -L --max-time 10 https://$TEST_HOST/; elif command -v wget >/dev/null 2>&1; then wget -S -O - https://$TEST_HOST/; elif command -v nc >/dev/null 2>&1; then nc -z -w 10 $TEST_HOST 443; else echo 'no curl, wget, or nc on Android device'; exit 21; fi"
}

adb_common_preflight() {
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
  mkdir -p "$SAME_DIR"
  printf '%s\n' "$android_ip" >"$SAME_DIR/adb-android-ip"
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
}

adb_check() {
  local android_ip
  adb_common_preflight
  android_ip="$(cat "$SAME_DIR/adb-android-ip" 2>/dev/null || true)"
  section "adb https probe"
  adb_https_probe

  section "host logs"
  if [[ -n "$android_ip" ]]; then
    printf 'expect Android source IP in dnsmasq log: %s\n' "$android_ip"
  fi
  /usr/bin/tail -n 120 "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null || true
  /usr/bin/tail -n 120 "$SAME_DIR/logs/mihomo.log" 2>/dev/null || true
}

log_line_count() {
  local file=$1
  if [[ -f "$file" ]]; then
    /usr/bin/wc -l <"$file" | /usr/bin/tr -d ' '
  else
    printf '0\n'
  fi
}

wait_for_policy_option() {
  local group=$1 option=$2 output="$SAME_DIR/policies-wait.json" error="$SAME_DIR/policies-wait.err"
  for _ in $(seq 1 50); do
    if "$OMG_BIN" policies --config "$CONFIG_TUN" --format json >"$output" 2>"$error" &&
      grep -Fq -- "\"name\": \"$group\"" "$output" &&
      grep -Fq -- "\"$option\"" "$output"; then
      return 0
    fi
    sleep 0.2
  done
  echo "policy group $group did not include option $option" >&2
  cat "$output" >&2 || true
  cat "$error" >&2 || true
  /usr/bin/tail -n 120 "$SAME_DIR/logs/mihomo.log" >&2 || true
  exit 1
}

wait_for_tun_policy_log_since() {
  local policy=$1 start_line=$2 log_file="$SAME_DIR/logs/mihomo.log"
  for _ in $(seq 1 30); do
    if [[ -f "$log_file" ]] &&
      /usr/bin/tail -n +"$((start_line + 1))" "$log_file" |
        grep -F -- "--> $TEST_HOST:443" |
        grep -Fq -- "using TunEgress[$policy]"; then
      printf 'same-LAN TUN policy log observed for %s:443 using TunEgress[%s]\n' "$TEST_HOST" "$policy"
      return 0
    fi
    sleep 1
  done
  printf 'mihomo did not log same-LAN TUN traffic for %s:443 using TunEgress[%s]\n' "$TEST_HOST" "$policy" >&2
  /usr/bin/tail -n 120 "$log_file" >&2 || true
  exit 1
}

assert_egress_proxy_unused() {
  if [[ -s "$SAME_DIR/egress/proxy.log" ]]; then
    echo "TunEgress DIRECT unexpectedly used the controlled proxy" >&2
    cat "$SAME_DIR/egress/proxy.log" >&2
    exit 1
  fi
}

assert_egress_proxy_used() {
  if ! grep -Fq -- "CONNECT $TEST_HOST:443" "$SAME_DIR/egress/proxy.log" 2>/dev/null; then
    printf 'controlled proxy did not observe CONNECT %s:443\n' "$TEST_HOST" >&2
    cat "$SAME_DIR/egress/proxy.log" >&2 || true
    exit 1
  fi
}

adb_check_imported_egress() {
  local before_log
  require_egress_probe_running
  adb_common_preflight

  section "providers and policies"
  wait_for_policy_option TunEgress egress-proxy
  "$OMG_BIN" providers --config "$CONFIG_TUN" --format json >"$SAME_DIR/tun-egress-providers.json"
  grep -Fq '"name": "tun-egress-provider"' "$SAME_DIR/tun-egress-providers.json"
  grep -Fq '"name": "egress-proxy"' "$SAME_DIR/tun-egress-providers.json"

  mkdir -p "$SAME_DIR/egress"
  : >"$SAME_DIR/egress/proxy.log"

  section "adb https probe TunEgress DIRECT"
  before_log="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  adb_https_probe
  wait_for_tun_policy_log_since DIRECT "$before_log"
  assert_egress_proxy_unused

  section "select TunEgress egress-proxy"
  "$OMG_BIN" policy-select --config "$CONFIG_TUN" --group TunEgress --policy egress-proxy --format json

  section "adb https probe TunEgress egress-proxy"
  before_log="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  adb_https_probe
  wait_for_tun_policy_log_since egress-proxy "$before_log"
  assert_egress_proxy_used

  section "host logs"
  /usr/bin/tail -n 120 "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null || true
  /usr/bin/tail -n 120 "$SAME_DIR/logs/mihomo.log" 2>/dev/null || true
  /usr/bin/tail -n 120 "$SAME_DIR/egress/proxy.log" 2>/dev/null || true
}

start_tun() {
  require_macos
  write_config
  build_omg
  run_root start
}

start_tun_imported_egress() {
  require_macos
  IMPORTED_EGRESS_ENABLED=true
  write_config
  build_omg
  build_egress_probe
  start_egress_probe
  if ! run_root start; then
    stop_egress_probe
    exit 1
  fi
}

stop_smoke() {
  require_macos
  write_config
  build_omg
  run_root stop
  stop_egress_probe
}

case "${1:-}" in
  start-tun)
    start_tun
    ;;
  start-tun-imported-egress)
    start_tun_imported_egress
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
  adb-check-imported-egress)
    adb_check_imported_egress
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
