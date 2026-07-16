#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WIFI_DHCP_ENABLED="${OMG_SAME_WIFI_DHCP_ENABLED:-false}"
case "$WIFI_DHCP_ENABLED" in
  1|true|TRUE|yes|YES)
    SAME_DIR="$ROOT/runtime/same-wifi-dhcp"
    SMOKE_LABEL="same-WiFi DHCP"
    ;;
  *)
    SAME_DIR="$ROOT/runtime/same-lan"
    SMOKE_LABEL="same-LAN"
    ;;
esac
CONFIG_TUN="$SAME_DIR/config-tun.yaml"
OMG_BIN="$ROOT/bin/omg"
NETWORK_BIN="$ROOT/bin/opensurge-network"
EGRESS_PROBE_BIN="$SAME_DIR/egress-probe"
EGRESS_PROBE_PID_FILE="$SAME_DIR/egress-probe.pid"
EGRESS_PROVIDER="$SAME_DIR/tun-egress-provider.yaml"
EGRESS_PROFILE_TEMPLATE="$ROOT/tests/lab/mihomo-profile.imported-tun-egress.yaml"
EGRESS_PROFILE="$SAME_DIR/mihomo-profile.imported-tun-egress.yaml"
DEVICE_POLICY_PROFILE_TEMPLATE="$ROOT/tests/same-lan/mihomo-profile.same-wifi-device-policy.yaml"
DEVICE_POLICY_PROFILE="$SAME_DIR/mihomo-profile.same-wifi-device-policy.yaml"
DEVICE_POLICY_FILE="$SAME_DIR/device-policy.json"
EGRESS_ORIGIN_PORT="${OMG_SAME_WIFI_DHCP_TUN_EGRESS_ORIGIN_PORT:-${OMG_SAME_LAN_TUN_EGRESS_ORIGIN_PORT:-19093}}"
EGRESS_PROXY_PORT="${OMG_SAME_WIFI_DHCP_TUN_EGRESS_PROXY_PORT:-${OMG_SAME_LAN_TUN_EGRESS_PROXY_PORT:-19094}}"
EGRESS_PROVIDER_URL="http://127.0.0.1:$EGRESS_ORIGIN_PORT/tun-egress-provider.yaml"
EGRESS_UPSTREAM_HTTP_PROXY="${OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY:-${OMG_SAME_LAN_TUN_EGRESS_UPSTREAM_HTTP_PROXY:-}}"
WIFI_DHCP_FORWARDING_FILE="$SAME_DIR/ip-forwarding-before"

IFACE="${OMG_SAME_WIFI_DHCP_IFACE:-${OMG_SAME_LAN_IFACE:-}}"
MAC_IP="${OMG_SAME_WIFI_DHCP_MAC_IP:-${OMG_SAME_LAN_MAC_IP:-}}"
TEST_HOST="${OMG_SAME_WIFI_DHCP_TEST_HOST:-${OMG_SAME_LAN_TEST_HOST:-example.com}}"
ADB_BIN="${ADB:-adb}"
ADB_SERIAL="${OMG_SAME_WIFI_DHCP_ADB_SERIAL:-${OMG_SAME_LAN_ADB_SERIAL:-}}"
IMPORTED_EGRESS_ENABLED="${OMG_SAME_LAN_IMPORTED_EGRESS:-false}"
WIFI_DHCP_NETWORK_SERVICE="${OMG_SAME_WIFI_DHCP_NETWORK_SERVICE:-Wi-Fi}"
WIFI_DHCP_ROUTER_DHCP_DISABLED="${OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED:-}"
WIFI_DHCP_PROTECTED_IPS="${OMG_SAME_WIFI_DHCP_PROTECTED_IPS:-}"
WIFI_DHCP_RANGE_START="${OMG_SAME_WIFI_DHCP_RANGE_START:-}"
WIFI_DHCP_RANGE_END="${OMG_SAME_WIFI_DHCP_RANGE_END:-}"
DEVICE_POLICY_ENABLED="${OMG_SAME_WIFI_DEVICE_POLICY_ENABLED:-false}"
DEVICE_ONE_ID="${OMG_SAME_WIFI_DEVICE_ONE_ID:-device-one}"
DEVICE_ONE_MAC="${OMG_SAME_WIFI_DEVICE_ONE_MAC:-}"
DEVICE_ONE_IP="${OMG_SAME_WIFI_DEVICE_ONE_IP:-}"
DEVICE_ONE_ADB_SERIAL="${OMG_SAME_WIFI_DEVICE_ONE_ADB_SERIAL:-}"
DEVICE_TWO_ID="${OMG_SAME_WIFI_DEVICE_TWO_ID:-device-two}"
DEVICE_TWO_MAC="${OMG_SAME_WIFI_DEVICE_TWO_MAC:-}"
DEVICE_TWO_IP="${OMG_SAME_WIFI_DEVICE_TWO_IP:-}"
DEVICE_TWO_ADB_SERIAL="${OMG_SAME_WIFI_DEVICE_TWO_ADB_SERIAL:-}"
DEVICE_DEFAULT_HOST="${OMG_SAME_WIFI_DEVICE_DEFAULT_HOST:-www.example.org}"
WIFI_DHCP_ROUTER_DHCP_RESTORED="${OMG_SAME_WIFI_DHCP_ROUTER_DHCP_RESTORED:-}"
WIFI_DHCP_CLIENTS_AUTOMATIC="${OMG_SAME_WIFI_DHCP_CLIENTS_AUTOMATIC:-}"

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
  start-wifi-dhcp-imported-egress
                               start the high-risk same-WiFi DHCP imported-egress fixture
  adb-check-wifi-dhcp-imported-egress
                               prove DHCP lease, DNS/TUN, provider, policy switch, and controlled egress
  start-wifi-dhcp-device-policy
                               start exact two-device reservations and per-device selectors
  adb-check-wifi-dhcp-device-policy
                               verify two ADB clients, independent selectors, rule slot, and UDP fail-closed
  verify-wifi-dhcp-device-policy-recovery
                               after router recovery, verify Mac/client DHCP and Internet recovery
  write-config                 write the mode-specific runtime config

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
  OMG_SAME_WIFI_DHCP_ENABLED=true
  OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed
  OMG_SAME_WIFI_DHCP_PROTECTED_IPS=192.168.1.101
  OMG_SAME_WIFI_DHCP_RANGE_START=192.168.1.120
  OMG_SAME_WIFI_DHCP_RANGE_END=192.168.1.199
  OMG_SAME_WIFI_DHCP_NETWORK_SERVICE=Wi-Fi
  OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY=192.168.1.101:8080
  OMG_SAME_WIFI_DEVICE_ONE_MAC=aa:bb:cc:dd:ee:01
  OMG_SAME_WIFI_DEVICE_ONE_IP=192.168.1.121
  OMG_SAME_WIFI_DEVICE_ONE_ADB_SERIAL=<adb-serial>
  OMG_SAME_WIFI_DEVICE_TWO_MAC=aa:bb:cc:dd:ee:02
  OMG_SAME_WIFI_DEVICE_TWO_IP=192.168.1.122
  OMG_SAME_WIFI_DEVICE_TWO_ADB_SERIAL=<adb-serial>
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

wifi_dhcp_enabled() {
  flag_enabled "$WIFI_DHCP_ENABLED"
}

device_policy_enabled() {
  flag_enabled "$DEVICE_POLICY_ENABLED"
}

sed_escape() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

lowercase() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

require_macos() {
  if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "same-LAN and same-WiFi DHCP smoke targets macOS only" >&2
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

hydrate_wifi_dhcp_runtime_interface() {
  local config_iface config_ip
  wifi_dhcp_enabled || return 0
  [[ -f "$CONFIG_TUN" ]] || return 0
  config_iface="$(/usr/bin/awk '
    $0 == "gateway:" { in_gateway = 1; next }
    in_gateway && /^[^[:space:]]/ { exit }
    in_gateway && $1 == "interface:" { gsub(/"/, "", $2); print $2; exit }
  ' "$CONFIG_TUN")"
  config_ip="$(/usr/bin/awk '
    $0 == "gateway:" { in_gateway = 1; next }
    in_gateway && /^[^[:space:]]/ { exit }
    in_gateway && $1 == "lan_ip:" { gsub(/"/, "", $2); print $2; exit }
  ' "$CONFIG_TUN")"
  if [[ -n "$config_iface" && -n "$config_ip" ]]; then
    IFACE="$config_iface"
    MAC_IP="$config_ip"
  fi
}

resolve_wifi_dhcp_range() {
  local mac_ip=$1 prefix
  prefix="${mac_ip%.*}"
  if [[ -z "$WIFI_DHCP_RANGE_START" ]]; then
    WIFI_DHCP_RANGE_START="$prefix.120"
  fi
  if [[ -z "$WIFI_DHCP_RANGE_END" ]]; then
    WIFI_DHCP_RANGE_END="$prefix.199"
  fi
}

ip_is_in_wifi_dhcp_range() {
  local ip=$1 start_octet end_octet ip_octet
  [[ "${ip%.*}" == "${WIFI_DHCP_RANGE_START%.*}" ]] || return 1
  [[ "${WIFI_DHCP_RANGE_START%.*}" == "${WIFI_DHCP_RANGE_END%.*}" ]] || return 1
  start_octet="${WIFI_DHCP_RANGE_START##*.}"
  end_octet="${WIFI_DHCP_RANGE_END##*.}"
  ip_octet="${ip##*.}"
  [[ "$start_octet" =~ ^[0-9]+$ && "$end_octet" =~ ^[0-9]+$ && "$ip_octet" =~ ^[0-9]+$ ]] || return 1
  (( 10#$ip_octet >= 10#$start_octet && 10#$ip_octet <= 10#$end_octet ))
}

require_wifi_dhcp_start_preflight() {
  local iface mac_ip info ip upstream_host upstream_port
  wifi_dhcp_enabled || return 0
  if [[ "$WIFI_DHCP_ROUTER_DHCP_DISABLED" != "confirmed" ]]; then
    echo "same-WiFi DHCP requires OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed after router DHCP is disabled" >&2
    exit 1
  fi
  if [[ -z "$WIFI_DHCP_PROTECTED_IPS" ]]; then
    echo "same-WiFi DHCP requires OMG_SAME_WIFI_DHCP_PROTECTED_IPS; include every static address that must never receive a lease" >&2
    exit 1
  fi
  if [[ "$EGRESS_UPSTREAM_HTTP_PROXY" != *:* ]]; then
    echo "same-WiFi DHCP requires OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY=<protected-lan-http-proxy-host:port> so the controlled proxy cannot re-enter TUN" >&2
    exit 1
  fi
  upstream_host="${EGRESS_UPSTREAM_HTTP_PROXY%:*}"
  upstream_port="${EGRESS_UPSTREAM_HTTP_PROXY##*:}"
  if [[ -z "$upstream_host" || ! "$upstream_port" =~ ^[0-9]+$ ]] ||
    ! /usr/bin/nc -z "$upstream_host" "$upstream_port"; then
    echo "same-WiFi DHCP controlled-proxy upstream is not reachable: $EGRESS_UPSTREAM_HTTP_PROXY" >&2
    exit 1
  fi
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  resolve_wifi_dhcp_range "$mac_ip"
  info="$(/usr/sbin/networksetup -getinfo "$WIFI_DHCP_NETWORK_SERVICE" 2>/dev/null || true)"
  if [[ "$info" != *"Manual Configuration"* || "$info" != *"IP address: $mac_ip"* ]]; then
    echo "same-WiFi DHCP requires $WIFI_DHCP_NETWORK_SERVICE to keep Mac $mac_ip as a manual IPv4 address before start" >&2
    exit 1
  fi
  if ip_is_in_wifi_dhcp_range "$mac_ip"; then
    echo "same-WiFi DHCP range $WIFI_DHCP_RANGE_START-$WIFI_DHCP_RANGE_END includes the Mac gateway $mac_ip" >&2
    exit 1
  fi
  local IFS=',' protected
  read -r -a protected <<< "$WIFI_DHCP_PROTECTED_IPS"
  for ip in "${protected[@]}"; do
    ip="${ip//[[:space:]]/}"
    [[ -n "$ip" ]] || continue
    if ip_is_in_wifi_dhcp_range "$ip"; then
      echo "same-WiFi DHCP range $WIFI_DHCP_RANGE_START-$WIFI_DHCP_RANGE_END includes protected static address $ip" >&2
      exit 1
    fi
  done
}

require_wifi_device_policy_preflight() {
  device_policy_enabled || return 0
  wifi_dhcp_enabled || { echo "device-policy real gate requires same-WiFi DHCP mode" >&2; exit 1; }
  local value
  for value in "$DEVICE_ONE_ID" "$DEVICE_TWO_ID"; do
    [[ "$value" =~ ^[A-Za-z0-9_-]+$ ]] || { echo "device IDs may contain only letters, numbers, underscore, or hyphen" >&2; exit 1; }
  done
  [[ "$DEVICE_ONE_ID" != "$DEVICE_TWO_ID" ]] || { echo "device IDs must be distinct" >&2; exit 1; }
  for value in "$DEVICE_ONE_MAC" "$DEVICE_TWO_MAC"; do
    [[ "$value" =~ ^([[:xdigit:]]{2}:){5}[[:xdigit:]]{2}$ ]] || { echo "two exact Wi-Fi MAC addresses are required for the device-policy gate" >&2; exit 1; }
  done
  [[ "$(lowercase "$DEVICE_ONE_MAC")" != "$(lowercase "$DEVICE_TWO_MAC")" ]] || { echo "device Wi-Fi MAC addresses must be distinct" >&2; exit 1; }
  [[ -n "$DEVICE_ONE_IP" && -n "$DEVICE_TWO_IP" && "$DEVICE_ONE_IP" != "$DEVICE_TWO_IP" ]] || { echo "two distinct reservation IPv4 addresses are required" >&2; exit 1; }
  ip_is_in_wifi_dhcp_range "$DEVICE_ONE_IP" || { echo "$DEVICE_ONE_IP is outside the same-WiFi DHCP pool" >&2; exit 1; }
  ip_is_in_wifi_dhcp_range "$DEVICE_TWO_IP" || { echo "$DEVICE_TWO_IP is outside the same-WiFi DHCP pool" >&2; exit 1; }
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

render_wifi_device_policy_fixture() {
  local mac_one mac_two
  mac_one="$(lowercase "$DEVICE_ONE_MAC")"
  mac_two="$(lowercase "$DEVICE_TWO_MAC")"
  [[ -f "$DEVICE_POLICY_PROFILE_TEMPLATE" ]] || { echo "missing device-policy profile template" >&2; exit 1; }
  sed "s|__CONTROLLED_PROXY_PORT__|$EGRESS_PROXY_PORT|g" "$DEVICE_POLICY_PROFILE_TEMPLATE" >"$DEVICE_POLICY_PROFILE"
  cat >"$DEVICE_POLICY_FILE" <<EOF
{
  "templates": [],
  "rule_sets": [],
  "profiles": [
    {
      "id": "controlled-first",
      "default_policies": ["same-wifi-controlled", "DIRECT"],
      "on_unsupported": "reject",
      "rules": [
        {
          "id": "policy-test",
          "match": {"domains": ["$TEST_HOST"], "protocols": ["tcp"], "ports": ["443"]},
          "policies": ["DIRECT", "same-wifi-controlled"],
          "on_unsupported": "reject"
        }
      ]
    },
    {
      "id": "direct-first",
      "default_policies": ["DIRECT", "same-wifi-controlled"],
      "on_unsupported": "reject"
    }
  ],
  "devices": [
    {"id": "$DEVICE_ONE_ID", "mac": "$mac_one", "ipv4": "$DEVICE_ONE_IP", "profile": "controlled-first", "egress_mode": "dedicated"},
    {"id": "$DEVICE_TWO_ID", "mac": "$mac_two", "ipv4": "$DEVICE_TWO_IP", "profile": "direct-first", "egress_mode": "dedicated"}
  ]
}
EOF
}

write_config() {
  local iface mac_ip profile_mode profile_path device_policy_path upstream_proxy_enabled gateway_mode dhcp_enabled range_start range_end domain runtime_dir anchor_name
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  if [[ -z "$iface" || -z "$mac_ip" ]]; then
    echo "could not resolve same-LAN interface or Mac IPv4 address" >&2
    exit 1
  fi

  mkdir -p "$SAME_DIR"
  profile_mode="managed"
  profile_path=""
  device_policy_path=""
  upstream_proxy_enabled="$UPSTREAM_PROXY_ENABLED"
  gateway_mode="same_lan"
  dhcp_enabled=false
  range_start="192.168.50.100"
  range_end="192.168.50.200"
  domain="same-lan"
  runtime_dir="runtime/same-lan"
  anchor_name="com.apple/open_mihomo_gateway_same_lan"
  if wifi_dhcp_enabled; then
    resolve_wifi_dhcp_range "$mac_ip"
    gateway_mode="same_wifi_dhcp"
    dhcp_enabled=true
    range_start="$WIFI_DHCP_RANGE_START"
    range_end="$WIFI_DHCP_RANGE_END"
    domain="same-wifi-dhcp"
    runtime_dir="runtime/same-wifi-dhcp"
    anchor_name="com.apple/open_mihomo_gateway_same_wifi_dhcp"
  fi
  if imported_egress_enabled; then
    if flag_enabled "$UPSTREAM_PROXY_ENABLED"; then
      echo "same-LAN imported egress uses its own controlled provider/proxy; leave OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED unset" >&2
      exit 1
    fi
    if device_policy_enabled; then
      require_wifi_device_policy_preflight
      render_wifi_device_policy_fixture
      profile_path="./$(basename "$DEVICE_POLICY_PROFILE")"
      device_policy_path="./$(basename "$DEVICE_POLICY_FILE")"
    else
      render_tun_egress_profile
      profile_path="./$(basename "$EGRESS_PROFILE")"
    fi
    profile_mode="imported"
    # Imported profile paths are resolved from CONFIG_TUN's directory. Keep
    # this path local to that mode-specific runtime directory.
    upstream_proxy_enabled=false
  fi

  cat >"$CONFIG_TUN" <<EOF
gateway:
  mode: "$gateway_mode"
  interface: "$iface"
  lan_ip: "$mac_ip"
  upstream_interface: "$iface"

dhcp:
  binary: "./runtime/tools/bin/dnsmasq"
  enabled: $dhcp_enabled
  range_start: "$range_start"
  range_end: "$range_end"
  lease_time: "30m"
  domain: "$domain"

device_policy:
  file: "$device_policy_path"
  protected_ipv4: "$WIFI_DHCP_PROTECTED_IPS"

dns:
  listen: "$mac_ip"
  port: 53
  upstream: "127.0.0.1#1053"

mihomo:
  binary: "./runtime/tools/bin/mihomo"
  config: "./$runtime_dir/mihomo.yaml"
  profile_mode: "$profile_mode"
  profile: "$profile_path"
  mixed_port: 17890
  redir_port: 0
  api_addr: "127.0.0.1:19090"
  secret: ""

pf:
  anchor_name: "$anchor_name"
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
  dir: "./$runtime_dir"
EOF

  section "config"
  printf 'wrote %s\n' "${CONFIG_TUN#$ROOT/}"
  printf '%s interface=%s mac_ip=%s test_host=%s\n' "$SMOKE_LABEL" "$iface" "$mac_ip" "$TEST_HOST"
  if wifi_dhcp_enabled; then
    printf 'DHCP range=%s-%s protected=%s\n' "$WIFI_DHCP_RANGE_START" "$WIFI_DHCP_RANGE_END" "$WIFI_DHCP_PROTECTED_IPS"
  fi
  if imported_egress_enabled; then
    if device_policy_enabled; then
      printf 'device policy=%s profile=%s proxy=127.0.0.1:%s\n' \
        "${DEVICE_POLICY_FILE#$ROOT/}" "${DEVICE_POLICY_PROFILE#$ROOT/}" "$EGRESS_PROXY_PORT"
    else
      printf 'imported egress profile=%s provider=%s proxy=127.0.0.1:%s\n' \
        "${EGRESS_PROFILE#$ROOT/}" "$EGRESS_PROVIDER_URL" "$EGRESS_PROXY_PORT"
    fi
  fi
}

build_omg() {
  section "build"
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$OMG_BIN" ./cmd/omg
  GOCACHE="${GOCACHE:-/private/tmp/omg-go-cache}" go build -o "$NETWORK_BIN" ./cmd/opensurge-network
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
  local -a probe_args
  section "start egress probe"
  stop_egress_probe
  mkdir -p "$SAME_DIR/logs"
  rm -rf "$SAME_DIR/egress"
  log_file="$SAME_DIR/logs/egress-probe.log"
  probe_args=(
    --origin "127.0.0.1:$EGRESS_ORIGIN_PORT"
    --proxy "127.0.0.1:$EGRESS_PROXY_PORT"
    --provider-file "$EGRESS_PROVIDER"
    --provider-path "/tun-egress-provider.yaml"
    --log-dir "$SAME_DIR/egress"
  )
  if [[ -n "$EGRESS_UPSTREAM_HTTP_PROXY" ]]; then
    probe_args+=(--upstream-http-proxy "$EGRESS_UPSTREAM_HTTP_PROXY")
  fi
  nohup "$EGRESS_PROBE_BIN" "${probe_args[@]}" >"$log_file" 2>&1 < /dev/null &
  pid=$!
  printf '%s\n' "$pid" >"$EGRESS_PROBE_PID_FILE"
  for _ in $(seq 1 50); do
    if grep -Fq READY "$log_file" 2>/dev/null &&
      /usr/bin/nc -z 127.0.0.1 "$EGRESS_ORIGIN_PORT" &&
      /usr/bin/nc -z 127.0.0.1 "$EGRESS_PROXY_PORT"; then
      printf 'TUN egress probe ready: provider=%s proxy=127.0.0.1:%s upstream=%s\n' "$EGRESS_PROVIDER_URL" "$EGRESS_PROXY_PORT" "${EGRESS_UPSTREAM_HTTP_PROXY:-direct}"
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
    echo "imported egress probe is not running; start the matching same-LAN or same-WiFi runner first" >&2
    exit 1
  fi
  pid="$(cat "$EGRESS_PROBE_PID_FILE")"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    echo "imported egress probe pid is stale; restart the matching same-LAN or same-WiFi runner" >&2
    exit 1
  fi
}

assert_egress_probe_stopped() {
  if [[ -e "$EGRESS_PROBE_PID_FILE" ]]; then
    echo "same-WiFi DHCP egress probe PID file still exists after stop" >&2
    exit 1
  fi
  if /usr/bin/nc -z 127.0.0.1 "$EGRESS_ORIGIN_PORT" 2>/dev/null ||
    /usr/bin/nc -z 127.0.0.1 "$EGRESS_PROXY_PORT" 2>/dev/null; then
    echo "same-WiFi DHCP egress helper still listens after stop" >&2
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
    OMG_SAME_WIFI_DHCP_ENABLED="$WIFI_DHCP_ENABLED" \
    OMG_SAME_WIFI_DHCP_NETWORK_SERVICE="$WIFI_DHCP_NETWORK_SERVICE" \
    /bin/bash "$0" "__root_$command" "$@"
}

assert_wifi_dhcp_stop_cleanup() {
  local expected actual
  [[ ! -e "$SAME_DIR/state.json" ]] || { echo "same-WiFi DHCP runtime state still exists after stop" >&2; exit 1; }
  if /sbin/pfctl -s Anchors | /usr/bin/grep -Fq "com.apple/open_mihomo_gateway_same_wifi_dhcp"; then
    echo "same-WiFi DHCP PF anchor remains after stop" >&2
    exit 1
  fi
  if [[ ! -r "$WIFI_DHCP_FORWARDING_FILE" ]]; then
    echo "same-WiFi DHCP forwarding baseline is missing" >&2
    exit 1
  fi
  expected="$(cat "$WIFI_DHCP_FORWARDING_FILE")"
  actual="$(/usr/sbin/sysctl -n net.inet.ip.forwarding)"
  if [[ "$actual" != "$expected" ]]; then
    echo "same-WiFi DHCP stop did not restore IPv4 forwarding: expected $expected, got $actual" >&2
    exit 1
  fi
  if /usr/sbin/lsof -nP -iUDP:53 2>/dev/null | /usr/bin/grep -Fq dnsmasq ||
    /usr/sbin/lsof -nP -iUDP:1053 2>/dev/null | /usr/bin/grep -Fq mihomo ||
    /usr/sbin/lsof -nP -iTCP:17890 -sTCP:LISTEN 2>/dev/null | /usr/bin/grep -Fq mihomo; then
    echo "same-WiFi DHCP listener remains after stop" >&2
    exit 1
  fi
  printf 'same-WiFi DHCP stop cleanup verified\n'
}

root_stop() {
  require_macos
  section "stop"
  if wifi_dhcp_enabled; then
    "$OMG_BIN" stop --config "$CONFIG_TUN"
  else
    "$OMG_BIN" stop --config "$CONFIG_TUN" || true
  fi

  section "status"
  "$OMG_BIN" status --config "$CONFIG_TUN" || true

  section "host cleanup checks"
  /usr/sbin/sysctl -n net.inet.ip.forwarding || true
  /sbin/pfctl -s Anchors || true
  if wifi_dhcp_enabled; then
    assert_wifi_dhcp_stop_cleanup
  fi
}

root_start() {
  require_macos

  section "stop existing $SMOKE_LABEL smoke"
  "$OMG_BIN" stop --config "$CONFIG_TUN" || true

  if wifi_dhcp_enabled; then
    /usr/sbin/sysctl -n net.inet.ip.forwarding >"$WIFI_DHCP_FORWARDING_FILE"
    section "active competing DHCP probe"
    "$NETWORK_BIN" probe-dhcp --interface "$(resolve_interface)" --expect none --timeout 3s
  fi

  section "root doctor"
  env PATH="$PATH" "$OMG_BIN" doctor --config "$CONFIG_TUN"

  section "start $SMOKE_LABEL TUN"
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

root_restore_wifi_dhcp() {
  require_macos
  section "router DHCP OFFER proof"
  "$NETWORK_BIN" probe-dhcp --interface "$(resolve_interface)" --expect any --timeout 5s
  section "restore Mac automatic DHCP and DNS"
  "$NETWORK_BIN" restore-dhcp --service "$WIFI_DHCP_NETWORK_SERVICE"
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
  local explicit_proxy
  explicit_proxy="$(adb_shell 'settings get global http_proxy || true' | /usr/bin/tr -d '\r\n')"
  printf '%s\n' "$explicit_proxy"
  if wifi_dhcp_enabled; then
    case "$explicit_proxy" in
      ""|null|:0) ;;
      *)
        echo "Android explicit proxy is set to $explicit_proxy; clear it before same-WiFi DHCP validation" >&2
        exit 1
        ;;
    esac
  fi
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

assert_wifi_dhcp_lease() {
  local android_ip mac_ip
  android_ip="$(cat "$SAME_DIR/adb-android-ip" 2>/dev/null || true)"
  mac_ip="$(resolve_mac_ip "$(resolve_interface)")"
  resolve_wifi_dhcp_range "$mac_ip"
  if [[ -z "$android_ip" ]] || ! ip_is_in_wifi_dhcp_range "$android_ip"; then
    echo "Android IPv4 $android_ip is not inside same-WiFi DHCP range $WIFI_DHCP_RANGE_START-$WIFI_DHCP_RANGE_END" >&2
    exit 1
  fi
  section "DHCP lease"
  "$OMG_BIN" leases --config "$CONFIG_TUN" --format json >"$SAME_DIR/wifi-dhcp-leases.json"
  if ! /usr/bin/grep -Fq "\"ip\": \"$android_ip\"" "$SAME_DIR/wifi-dhcp-leases.json" ||
    ! /usr/bin/grep -Fq "DHCPACK" "$SAME_DIR/logs/dnsmasq.log" ||
    ! /usr/bin/grep -Fq "$android_ip" "$SAME_DIR/logs/dnsmasq.log"; then
    echo "OpenSurge DHCP did not prove an ACK and lease for Android $android_ip" >&2
    cat "$SAME_DIR/wifi-dhcp-leases.json" >&2 || true
    /usr/bin/tail -n 160 "$SAME_DIR/logs/dnsmasq.log" >&2 || true
    exit 1
  fi
  printf 'same-WiFi DHCP lease observed for Android %s via Mac %s\n' "$android_ip" "$mac_ip"
}

wait_for_wifi_dhcp_dns_log() {
  local android_ip=$1
  for _ in $(seq 1 15); do
    if /usr/bin/grep -F "$TEST_HOST" "$SAME_DIR/logs/dnsmasq.log" 2>/dev/null |
      /usr/bin/grep -Fq "from $android_ip"; then
      printf 'same-WiFi DHCP DNS log observed for %s from %s\n' "$TEST_HOST" "$android_ip"
      return 0
    fi
    sleep 1
  done
  echo "dnsmasq did not log $TEST_HOST from Android $android_ip" >&2
  /usr/bin/tail -n 160 "$SAME_DIR/logs/dnsmasq.log" >&2 || true
  exit 1
}

adb_check_imported_egress() {
  local before_log android_ip
  require_egress_probe_running
  adb_common_preflight
  android_ip="$(cat "$SAME_DIR/adb-android-ip" 2>/dev/null || true)"
  if wifi_dhcp_enabled; then
    assert_wifi_dhcp_lease
  fi

  section "providers and policies"
  wait_for_policy_option TunEgress egress-proxy
  "$OMG_BIN" providers --config "$CONFIG_TUN" --format json >"$SAME_DIR/tun-egress-providers.json"
  grep -Fq '"name": "tun-egress-provider"' "$SAME_DIR/tun-egress-providers.json"
  grep -Fq '"name": "egress-proxy"' "$SAME_DIR/tun-egress-providers.json"
  if wifi_dhcp_enabled; then
    "$OMG_BIN" provider-update --config "$CONFIG_TUN" --provider tun-egress-provider --format json >"$SAME_DIR/tun-egress-provider-update.json"
    grep -Fq '"provider": "tun-egress-provider"' "$SAME_DIR/tun-egress-provider-update.json"
    grep -Fq '"updated": true' "$SAME_DIR/tun-egress-provider-update.json"
    grep -Fq '"name": "egress-proxy"' "$SAME_DIR/tun-egress-provider-update.json"
    wait_for_policy_option TunEgress egress-proxy
  fi

  mkdir -p "$SAME_DIR/egress"
  : >"$SAME_DIR/egress/proxy.log"

  section "select TunEgress DIRECT"
  "$OMG_BIN" policy-select --config "$CONFIG_TUN" --group TunEgress --policy DIRECT --format json

  section "adb https probe TunEgress DIRECT"
  before_log="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  adb_https_probe
  wait_for_tun_policy_log_since DIRECT "$before_log"
  if wifi_dhcp_enabled; then
    wait_for_wifi_dhcp_dns_log "$android_ip"
  fi
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

device_adb_shell() {
  local serial=$1
  shift
  "$ADB_BIN" -s "$serial" shell "$@"
}

device_https_probe() {
  local serial=$1 host=$2
  device_adb_shell "$serial" "if command -v curl >/dev/null 2>&1; then curl -I -L --max-time 10 https://$host/; elif command -v wget >/dev/null 2>&1; then wget -S -O - https://$host/; elif command -v nc >/dev/null 2>&1; then nc -z -w 10 $host 443; else echo 'no curl, wget, or nc on Android device'; exit 21; fi"
}

device_client_preflight() {
  local id=$1 serial=$2 expected_mac=$3 expected_ip=$4 mac_ip actual_ip actual_mac route explicit_proxy expected_mac_lower
  [[ -n "$serial" ]] || { echo "$id requires an ADB serial" >&2; exit 1; }
  "$ADB_BIN" -s "$serial" get-state | grep -qx device || { echo "$id ADB device is not authorized: $serial" >&2; exit 1; }
  mac_ip="$(resolve_mac_ip "$(resolve_interface)")"
  actual_ip="$(device_adb_shell "$serial" 'ip -4 -o addr show dev wlan0 scope global' | awk 'NR == 1 { split($4, value, "/"); print value[1] }' | tr -d '\r')"
  actual_mac="$(device_adb_shell "$serial" 'cat /sys/class/net/wlan0/address' | tr -d '\r\n' | tr '[:upper:]' '[:lower:]')"
  route="$(device_adb_shell "$serial" 'ip -4 route get 1.1.1.1 2>/dev/null || ip route get 1.1.1.1 2>/dev/null' | tr -d '\r')"
  explicit_proxy="$(device_adb_shell "$serial" 'settings get global http_proxy || true' | tr -d '\r\n')"
  printf '%s serial=%s mac=%s ip=%s route=%s proxy=%s\n' "$id" "$serial" "$actual_mac" "$actual_ip" "$route" "$explicit_proxy"
  [[ "$actual_ip" == "$expected_ip" ]] || { echo "$id received $actual_ip, expected reservation $expected_ip" >&2; exit 1; }
  expected_mac_lower="$(lowercase "$expected_mac")"
  [[ "$actual_mac" == "$expected_mac_lower" ]] || { echo "$id wlan0 MAC $actual_mac does not match configured Wi-Fi MAC $expected_mac_lower" >&2; exit 1; }
  [[ "$route" == *"via $mac_ip"* ]] || { echo "$id effective route is not via Mac $mac_ip" >&2; exit 1; }
  case "$explicit_proxy" in ""|null|:0) ;; *) echo "$id explicit proxy must be disabled" >&2; exit 1 ;; esac
}

assert_device_lease_and_identity() {
  local id=$1 mac=$2 ip=$3 leases=$4 devices=$5
  grep -B 4 -A 5 -F "\"ip\": \"$ip\"" "$leases" | grep -Fiq "\"mac\": \"$mac\"" || { echo "lease did not bind $id $mac to $ip" >&2; exit 1; }
  grep -B 4 -A 30 -F "\"id\": \"$id\"" "$devices" | grep -Fq '"policy_identity_ready": true' || { echo "$id policy identity is not ready" >&2; exit 1; }
  grep -B 4 -A 30 -F "\"id\": \"$id\"" "$devices" | grep -Fq '"lease_match": true' || { echo "$id lease does not match applied policy" >&2; exit 1; }
  grep -F "$ip" "$SAME_DIR/logs/dnsmasq.log" | grep -Fiq "$mac" || { echo "dnsmasq log lacks exact ACK identity for $id" >&2; exit 1; }
}

wait_for_device_policy_log_since() {
  local source_ip=$1 group=$2 policy=$3 host=$4 start_line=$5 log_file="$SAME_DIR/logs/mihomo.log"
  for _ in $(seq 1 30); do
    if tail -n +"$((start_line + 1))" "$log_file" 2>/dev/null |
      grep -F -- "$source_ip" | grep -F -- "--> $host:443" | grep -Fq -- "using $group[$policy]"; then
      printf 'device policy log observed: %s -> %s via %s[%s]\n' "$source_ip" "$host" "$group" "$policy"
      return 0
    fi
    sleep 1
  done
  echo "missing source-specific device policy log for $source_ip -> $host using $group[$policy]" >&2
  tail -n 180 "$log_file" >&2 || true
  exit 1
}

assert_controlled_proxy_used_for() {
  local host=$1
  grep -Fq "CONNECT $host:443" "$SAME_DIR/egress/proxy.log" 2>/dev/null || { echo "controlled proxy did not observe CONNECT $host:443" >&2; exit 1; }
}

device_udp_probe() {
  local serial=$1
  device_adb_shell "$serial" "if command -v nc >/dev/null 2>&1; then printf x | nc -u -w 1 1.1.1.1 443 || true; elif toybox nc --help >/dev/null 2>&1; then printf x | toybox nc -u -w 1 1.1.1.1 443 || true; else echo 'no UDP-capable nc on Android device'; exit 21; fi"
}

wait_for_device_udp_reject() {
  local source_ip=$1 log_file="$SAME_DIR/logs/mihomo.log"
  for _ in $(seq 1 20); do
    if grep -E "\\[UDP\\].*$source_ip.*--> 1.1.1.1:443.*using REJECT" "$log_file" >/dev/null 2>&1; then
      if grep -E "\\[UDP\\].*$source_ip.*--> 1.1.1.1:443.*using DIRECT" "$log_file" >/dev/null 2>&1; then
        echo "UDP from $source_ip fell through to DIRECT" >&2; exit 1
      fi
      echo "UDP fail-closed REJECT observed for $source_ip"
      return 0
    fi
    sleep 1
  done
  echo "missing UDP fail-closed REJECT for $source_ip" >&2
  tail -n 180 "$log_file" >&2 || true
  exit 1
}

adb_check_wifi_dhcp_device_policy() {
  local before group_one_default="device/$DEVICE_ONE_ID/default" group_one_rule="device/$DEVICE_ONE_ID/policy-test" group_two_default="device/$DEVICE_TWO_ID/default"
  device_policy_enabled || { echo "set OMG_SAME_WIFI_DEVICE_POLICY_ENABLED=true" >&2; exit 1; }
  require_egress_probe_running
  device_client_preflight "$DEVICE_ONE_ID" "$DEVICE_ONE_ADB_SERIAL" "$DEVICE_ONE_MAC" "$DEVICE_ONE_IP"
  device_client_preflight "$DEVICE_TWO_ID" "$DEVICE_TWO_ADB_SERIAL" "$DEVICE_TWO_MAC" "$DEVICE_TWO_IP"
  "$OMG_BIN" leases --config "$CONFIG_TUN" --format json >"$SAME_DIR/device-policy-leases.json"
  "$OMG_BIN" devices --config "$CONFIG_TUN" --format json >"$SAME_DIR/device-policy-devices.json"
  assert_device_lease_and_identity "$DEVICE_ONE_ID" "$(lowercase "$DEVICE_ONE_MAC")" "$DEVICE_ONE_IP" "$SAME_DIR/device-policy-leases.json" "$SAME_DIR/device-policy-devices.json"
  assert_device_lease_and_identity "$DEVICE_TWO_ID" "$(lowercase "$DEVICE_TWO_MAC")" "$DEVICE_TWO_IP" "$SAME_DIR/device-policy-leases.json" "$SAME_DIR/device-policy-devices.json"
  wait_for_policy_option "$group_one_rule" same-wifi-controlled
  wait_for_policy_option "$group_one_default" same-wifi-controlled
  wait_for_policy_option "$group_two_default" same-wifi-controlled

  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_ONE_ADB_SERIAL" "$TEST_HOST"
  wait_for_device_policy_log_since "$DEVICE_ONE_IP" "$group_one_rule" DIRECT "$TEST_HOST" "$before"
  assert_egress_proxy_unused

  "$OMG_BIN" device-policy-select --config "$CONFIG_TUN" --device "$DEVICE_ONE_ID" --slot policy-test --policy same-wifi-controlled --format json
  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_ONE_ADB_SERIAL" "$TEST_HOST"
  wait_for_device_policy_log_since "$DEVICE_ONE_IP" "$group_one_rule" same-wifi-controlled "$TEST_HOST" "$before"
  assert_controlled_proxy_used_for "$TEST_HOST"

  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_ONE_ADB_SERIAL" "$DEVICE_DEFAULT_HOST"
  wait_for_device_policy_log_since "$DEVICE_ONE_IP" "$group_one_default" same-wifi-controlled "$DEVICE_DEFAULT_HOST" "$before"
  assert_controlled_proxy_used_for "$DEVICE_DEFAULT_HOST"

  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_TWO_ADB_SERIAL" "$DEVICE_DEFAULT_HOST"
  wait_for_device_policy_log_since "$DEVICE_TWO_IP" "$group_two_default" DIRECT "$DEVICE_DEFAULT_HOST" "$before"
  assert_egress_proxy_unused

  "$OMG_BIN" device-policy-select --config "$CONFIG_TUN" --device "$DEVICE_ONE_ID" --slot default --policy DIRECT --format json
  "$OMG_BIN" device-policy-select --config "$CONFIG_TUN" --device "$DEVICE_TWO_ID" --slot default --policy same-wifi-controlled --format json
  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_ONE_ADB_SERIAL" "$DEVICE_DEFAULT_HOST"
  wait_for_device_policy_log_since "$DEVICE_ONE_IP" "$group_one_default" DIRECT "$DEVICE_DEFAULT_HOST" "$before"
  assert_egress_proxy_unused
  : >"$SAME_DIR/egress/proxy.log"
  before="$(log_line_count "$SAME_DIR/logs/mihomo.log")"
  device_https_probe "$DEVICE_TWO_ADB_SERIAL" "$DEVICE_DEFAULT_HOST"
  wait_for_device_policy_log_since "$DEVICE_TWO_IP" "$group_two_default" same-wifi-controlled "$DEVICE_DEFAULT_HOST" "$before"
  assert_controlled_proxy_used_for "$DEVICE_DEFAULT_HOST"

  device_udp_probe "$DEVICE_TWO_ADB_SERIAL"
  wait_for_device_udp_reject "$DEVICE_TWO_IP"
  "$OMG_BIN" policies --config "$CONFIG_TUN" --format json >"$SAME_DIR/device-policy-live.json"
  grep -A 6 -F "\"name\": \"$group_one_default\"" "$SAME_DIR/device-policy-live.json" | grep -Fq '"selected": "DIRECT"'
  grep -A 6 -F "\"name\": \"$group_two_default\"" "$SAME_DIR/device-policy-live.json" | grep -Fq '"selected": "same-wifi-controlled"'
  echo "same-WiFi two-device policy gate passed for the active cooperative IPv4 run"
}

start_tun() {
  require_macos
  if wifi_dhcp_enabled; then
    echo "same-WiFi DHCP only supports the imported egress full runner; use start-wifi-dhcp-imported-egress" >&2
    exit 1
  fi
  write_config
  build_omg
  run_root start
}

start_tun_imported_egress() {
  require_macos
  require_wifi_dhcp_start_preflight
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

start_wifi_dhcp_imported_egress() {
  if ! wifi_dhcp_enabled; then
    echo "set OMG_SAME_WIFI_DHCP_ENABLED=true before starting the same-WiFi DHCP runner" >&2
    exit 1
  fi
  start_tun_imported_egress
}

start_wifi_dhcp_device_policy() {
  wifi_dhcp_enabled || { echo "set OMG_SAME_WIFI_DHCP_ENABLED=true" >&2; exit 1; }
  DEVICE_POLICY_ENABLED=true
  IMPORTED_EGRESS_ENABLED=true
  require_wifi_dhcp_start_preflight
  require_wifi_device_policy_preflight
  start_tun_imported_egress
}

adb_check_wifi_dhcp_imported_egress() {
  if ! wifi_dhcp_enabled; then
    echo "set OMG_SAME_WIFI_DHCP_ENABLED=true before running the same-WiFi DHCP ADB gate" >&2
    exit 1
  fi
  adb_check_imported_egress
}

verify_wifi_dhcp_device_policy_recovery() {
  wifi_dhcp_enabled || { echo "set OMG_SAME_WIFI_DHCP_ENABLED=true" >&2; exit 1; }
  [[ "$WIFI_DHCP_ROUTER_DHCP_RESTORED" == "confirmed" ]] || { echo "confirm router DHCP restoration with OMG_SAME_WIFI_DHCP_ROUTER_DHCP_RESTORED=confirmed" >&2; exit 1; }
  [[ "$WIFI_DHCP_CLIENTS_AUTOMATIC" == "confirmed" ]] || { echo "put both clients on automatic IP/DNS, then set OMG_SAME_WIFI_DHCP_CLIENTS_AUTOMATIC=confirmed" >&2; exit 1; }
  [[ -n "$DEVICE_ONE_ADB_SERIAL" && -n "$DEVICE_TWO_ADB_SERIAL" ]] || { echo "two ADB serials are required for recovery proof" >&2; exit 1; }
  hydrate_wifi_dhcp_runtime_interface
  build_omg
  run_root restore_wifi_dhcp
  sleep 8
  local iface info mac_ip router route ip
  iface="$(resolve_interface)"
  mac_ip="$(resolve_mac_ip "$iface")"
  info="$(/usr/sbin/networksetup -getinfo "$WIFI_DHCP_NETWORK_SERVICE")"
  [[ "$info" == *"DHCP Configuration"* ]] || { echo "Mac network service is not back on DHCP" >&2; exit 1; }
  /usr/sbin/ipconfig getpacket "$iface" | tee "$SAME_DIR/recovery-mac-dhcp-packet.txt" | grep -q 'server_identifier' || { echo "Mac has no DHCP server_identifier evidence" >&2; exit 1; }
  router="$(/sbin/route -n get default | awk '/gateway:/ {print $2; exit}')"
  [[ -n "$router" && "$router" != "$mac_ip" ]] || { echo "Mac default router was not restored" >&2; exit 1; }
  /usr/bin/curl --fail --silent --show-error --max-time 10 "https://$DEVICE_DEFAULT_HOST/" >/dev/null
  for pair in "$DEVICE_ONE_ID|$DEVICE_ONE_ADB_SERIAL" "$DEVICE_TWO_ID|$DEVICE_TWO_ADB_SERIAL"; do
    local id="${pair%%|*}" serial="${pair#*|}"
    ip="$(device_adb_shell "$serial" 'ip -4 -o addr show dev wlan0 scope global' | awk 'NR == 1 { split($4, value, "/"); print value[1] }' | tr -d '\r')"
    route="$(device_adb_shell "$serial" 'ip -4 route get 1.1.1.1 2>/dev/null || ip route get 1.1.1.1 2>/dev/null' | tr -d '\r')"
    printf '%s recovered ip=%s route=%s\n' "$id" "$ip" "$route" | tee -a "$SAME_DIR/recovery-clients.txt"
    [[ -n "$ip" && "$route" == *"via $router"* && "$route" != *"via $mac_ip"* ]] || { echo "$id did not recover the router DHCP path" >&2; exit 1; }
    device_https_probe "$serial" "$DEVICE_DEFAULT_HOST"
    device_adb_shell "$serial" 'dumpsys wifi | grep -i -E "DHCP|gateway|dns" | head -40 || true' >>"$SAME_DIR/recovery-clients.txt"
  done
  [[ ! -e "$SAME_DIR/state.json" ]] || { echo "OpenSurge runtime state remains after recovery" >&2; exit 1; }
  echo "same-WiFi device-policy DHCP recovery gate passed"
}

stop_smoke() {
  require_macos
  hydrate_wifi_dhcp_runtime_interface
  [[ -f "$CONFIG_TUN" ]] || write_config
  build_omg
  if ! run_root stop; then
    stop_egress_probe
    exit 1
  fi
  stop_egress_probe
  if wifi_dhcp_enabled; then
    assert_egress_probe_stopped
  fi
}

case "${1:-}" in
  start-tun)
    start_tun
    ;;
  start-tun-imported-egress)
    start_tun_imported_egress
    ;;
  start-wifi-dhcp-imported-egress)
    start_wifi_dhcp_imported_egress
    ;;
  start-wifi-dhcp-device-policy)
    start_wifi_dhcp_device_policy
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
  adb-check-wifi-dhcp-imported-egress)
    adb_check_wifi_dhcp_imported_egress
    ;;
  adb-check-wifi-dhcp-device-policy)
    adb_check_wifi_dhcp_device_policy
    ;;
  verify-wifi-dhcp-device-policy-recovery)
    verify_wifi_dhcp_device_policy_recovery
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
  __root_restore_wifi_dhcp)
    shift
    root_restore_wifi_dhcp "$@"
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
