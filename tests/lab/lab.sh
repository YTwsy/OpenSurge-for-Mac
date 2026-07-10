#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROXY_ENV="$ROOT/runtime/lab/proxy.env"
TOOLS_ROOT="$ROOT/runtime/tools"
if [[ -f "$PROXY_ENV" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$PROXY_ENV"
  set +a
fi
export PATH="$TOOLS_ROOT/lima/bin:$TOOLS_ROOT/bin:$PATH"
NETWORK_HELPER=/opt/open-mihomo-gateway/bin/omg-lab-network
SOCKET=/private/var/run/open-mihomo-gateway-lab.sock
INTERFACE_FILE=/private/var/run/open-mihomo-gateway-lab.interface
TEMPLATE="$ROOT/tests/lab/lima/client.yaml"
CONFIG_TEMPLATE="$ROOT/tests/lab/config.yaml.tmpl"
STATE_DIR="$ROOT/runtime/lab"
CONFIG="$STATE_DIR/config.yaml"
CLIENT_CONFIG="$STATE_DIR/client.yaml"
BINARY="$ROOT/bin/omg-lab"
EGRESS_PROBE_BINARY="$STATE_DIR/egress-probe"
EGRESS_PROVIDER="$STATE_DIR/tun-egress-provider.yaml"
EGRESS_ORIGIN_PORT="${OMG_LAB_TUN_EGRESS_ORIGIN_PORT:-19093}"
EGRESS_PROXY_PORT="${OMG_LAB_TUN_EGRESS_PROXY_PORT:-19094}"
EGRESS_PROVIDER_URL="http://127.0.0.1:$EGRESS_ORIGIN_PORT/tun-egress-provider.yaml"
LAN_IP=192.168.50.1
CLIENTS="${OMG_LAB_CLIENTS:-omg-lab-client-1 omg-lab-client-2}"
TEST_URL="${OMG_LAB_TEST_URL:-https://example.com/}"
LAB_MIHOMO_PROFILE="${OMG_LAB_MIHOMO_PROFILE:-}"
LAB_DEVICE_POLICY_FILE=""
TUN_EGRESS_PROFILE=0
EGRESS_PROBE_PID=""

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

require_installed_lab() {
  require_command limactl
  require_command dnsmasq
  require_command mihomo
  if [[ ! -x "$NETWORK_HELPER" ]]; then
    echo "lab network helper is not installed; run: make lab-install" >&2
    exit 1
  fi
}

require_cached_sudo() {
  if ! sudo -n true 2>/dev/null; then
    echo "gateway test requires a cached sudo credential; run 'sudo -v' in a terminal, then retry" >&2
    exit 1
  fi
}

ensure_lab_state_writable() {
  mkdir -p "$STATE_DIR"
  # The gateway itself runs as root and creates runtime logs/configuration.
  # Reclaim only the disposable lab directory before a new test so the
  # unprivileged egress fixture can write its own evidence files.
  sudo -n chown -R "$(id -u):$(id -g)" "$STATE_DIR"
  mkdir -p "$STATE_DIR/logs"
  [[ -w "$STATE_DIR" && -w "$STATE_DIR/logs" ]] || {
    echo "lab runtime directory is not writable: $STATE_DIR" >&2
    exit 1
  }
}

instance_dir() {
  printf '%s/%s\n' "${LIMA_HOME:-$HOME/.lima}" "$1"
}

start_network() {
  sudo -n "$NETWORK_HELPER" start
  [[ -S "$SOCKET" ]] || { echo "lab socket was not created" >&2; exit 1; }
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab interface state was not created" >&2; exit 1; }
}

lab_interface() {
  cat "$INTERFACE_FILE"
}

interfaces_with_lab_ip_except() {
  local allowed_iface iface
  allowed_iface="$1"
  for iface in $(/sbin/ifconfig -l); do
    if [[ "$iface" == "$allowed_iface" ]]; then
      continue
    fi
    if /sbin/ifconfig "$iface" 2>/dev/null | grep -q "inet $LAN_IP "; then
      printf '%s\n' "$iface"
    fi
  done
}

require_unique_lab_ip() {
  local iface conflicts
  iface="$1"
  conflicts="$(interfaces_with_lab_ip_except "$iface")"
  if [[ -n "$conflicts" ]]; then
    echo "lab LAN IP $LAN_IP is already configured on non-lab interface(s):" >&2
    printf '%s\n' "$conflicts" >&2
    echo "remove the duplicate address before running the lab, for example with make real-device-stop or sudo ifconfig <iface> inet $LAN_IP delete" >&2
    exit 1
  fi
}

upstream_interface() {
  /sbin/route -n get default | awk '/interface:/ { print $2; exit }'
}

sed_escape() {
  printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

shell_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

write_proxy_exports() {
  local indent=$1 name value
  case "${OMG_LAB_VM_PROXY:-0}" in
    1|true|TRUE|yes|YES) ;;
    *) return 0 ;;
  esac
  for name in HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy FTP_PROXY ftp_proxy grpc_proxy NO_PROXY no_proxy; do
    value="${!name:-}"
    if [[ -n "$value" ]]; then
      printf '%sexport %s=%s\n' "$indent" "$name" "$(shell_quote "$value")"
    fi
  done
}

resolve_lab_profile() {
  local profile=$1
  case "$profile" in
    /*) printf '%s\n' "$profile" ;;
    *) printf '%s/%s\n' "$ROOT" "$profile" ;;
  esac
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
  local source=$1 destination=$2 host
  host="$(url_host "$TEST_URL")"
  write_tun_egress_provider
  sed \
    -e "s|__TUN_EGRESS_PROVIDER_URL__|$(sed_escape "$EGRESS_PROVIDER_URL")|g" \
    -e "s|__TUN_EGRESS_HOST__|$(sed_escape "$host")|g" \
    "$source" >"$destination"
}

write_config() {
  local mode iface upstream dnsmasq_bin mihomo_bin runtime_dir dns_upstream_line device_policy_file
  local profile_mode profile_path profile_source
	mode="${1:-off}"
  TUN_EGRESS_PROFILE=0
	iface="$(lab_interface)"
	upstream="$(upstream_interface)"
	dnsmasq_bin="$(command -v dnsmasq)"
	mihomo_bin="$(command -v mihomo)"
  runtime_dir="$STATE_DIR"
  device_policy_file="$LAB_DEVICE_POLICY_FILE"
  dns_upstream_line=""
  profile_mode="managed"
  profile_path=""
  if [[ -n "$LAB_MIHOMO_PROFILE" ]]; then
    profile_mode="imported"
    profile_source="$(resolve_lab_profile "$LAB_MIHOMO_PROFILE")"
    [[ -f "$profile_source" ]] || { echo "mihomo profile not found: $profile_source" >&2; exit 1; }
    profile_path="$profile_source"
    if [[ "$profile_source" == "$ROOT/tests/lab/mihomo-profile.imported-tun-egress.yaml" ]]; then
      mkdir -p "$STATE_DIR"
      profile_path="$STATE_DIR/$(basename "$profile_source")"
      render_tun_egress_profile "$profile_source" "$profile_path"
      TUN_EGRESS_PROFILE=1
    elif [[ "$profile_source" == "$ROOT/tests/lab/mihomo-profile.imported-tun.yaml" ]]; then
      mkdir -p "$STATE_DIR"
      profile_path="$STATE_DIR/$(basename "$profile_source")"
      cp "$profile_source" "$profile_path"
    fi
  fi
  case "$mode" in
    off) ;;
    tun) dns_upstream_line='  upstream: "127.0.0.1#1053"' ;;
    *) echo "unknown transparent mode: $mode" >&2; exit 2 ;;
  esac

  case "$iface" in
    bridge*) ;;
    *) echo "refusing non-bridge lab interface: $iface" >&2; exit 1 ;;
  esac
  /sbin/ifconfig "$iface" | grep -q 'member: vmenet'
  /sbin/ifconfig "$iface" | grep -q "inet $LAN_IP "
  require_unique_lab_ip "$iface"
  [[ "$iface" != "$upstream" ]] || { echo "lab and upstream interfaces must differ" >&2; exit 1; }

  mkdir -p "$STATE_DIR"
  sed \
	  -e "s|__LAB_INTERFACE__|$(sed_escape "$iface")|g" \
	  -e "s|__UPSTREAM_INTERFACE__|$(sed_escape "$upstream")|g" \
	  -e "s|__DNSMASQ_BINARY__|$(sed_escape "$dnsmasq_bin")|g" \
	  -e "s|__MIHOMO_BINARY__|$(sed_escape "$mihomo_bin")|g" \
    -e "s|__MIHOMO_PROFILE_MODE__|$(sed_escape "$profile_mode")|g" \
    -e "s|__MIHOMO_PROFILE__|$(sed_escape "$profile_path")|g" \
    -e "s|__DEVICE_POLICY_FILE__|$(sed_escape "$device_policy_file")|g" \
    -e "s|__DNS_UPSTREAM_LINE__|$(sed_escape "$dns_upstream_line")|g" \
    -e "s|__TRANSPARENT_MODE__|$(sed_escape "$mode")|g" \
    -e "s|__RUNTIME_DIR__|$(sed_escape "$runtime_dir")|g" \
    "$CONFIG_TEMPLATE" >"$CONFIG"
}

write_client_config() {
  local line
  mkdir -p "$STATE_DIR"
  : >"$CLIENT_CONFIG"
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      *__PROXY_EXPORTS__*)
        write_proxy_exports "      "
        ;;
      *)
        printf '%s\n' "$line"
        ;;
    esac
  done <"$TEMPLATE" >"$CLIENT_CONFIG"
}

start_clients() {
  local client instance_yaml
  write_client_config
  for client in $CLIENTS; do
    instance_yaml="$(instance_dir "$client")/lima.yaml"
    if [[ -f "$instance_yaml" ]] && ! cmp -s "$instance_yaml" "$CLIENT_CONFIG"; then
      limactl stop "$client" || true
      limactl delete -f -y "$client"
    fi
    if [[ ! -d "$(instance_dir "$client")" ]]; then
      limactl create -y --name "$client" "$CLIENT_CONFIG"
    fi
    limactl start "$client"
    limactl shell "$client" -- true
  done
}

stop_clients() {
  local client
  for client in $CLIENTS; do
    if [[ -d "$(instance_dir "$client")" ]]; then
      limactl stop "$client" || true
    fi
  done
}

destroy_clients() {
  local client
  for client in $CLIENTS; do
    if [[ -d "$(instance_dir "$client")" ]]; then
      limactl stop "$client" || true
      limactl delete "$client"
    fi
  done
}

collect_artifacts() {
  local artifact_dir client
  artifact_dir="$ROOT/artifacts/lab/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$artifact_dir"
  cp "$CONFIG" "$artifact_dir/config.yaml" 2>/dev/null || true
  cp "$LAB_DEVICE_POLICY_FILE" "$artifact_dir/device-policy.json" 2>/dev/null || true
  cp "$LAB_MIHOMO_PROFILE" "$artifact_dir/device-policy-profile.yaml" 2>/dev/null || true
  cp "$EGRESS_PROVIDER" "$artifact_dir/tun-egress-provider.yaml" 2>/dev/null || true
  cp -R "$STATE_DIR/egress" "$artifact_dir/egress" 2>/dev/null || true
  cp -R "$STATE_DIR/logs" "$artifact_dir/logs" 2>/dev/null || true
  "$BINARY" status --config "$CONFIG" >"$artifact_dir/gateway-status.txt" 2>&1 || true
  "$BINARY" leases --config "$CONFIG" >"$artifact_dir/leases.txt" 2>&1 || true
  /sbin/ifconfig "$(lab_interface)" >"$artifact_dir/interface.txt" 2>&1 || true
  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status \
      >"$artifact_dir/$client.txt" 2>&1 || true
  done
  echo "Lab artifacts: $artifact_dir"
}

url_host() {
  local url host
  url="$1"
  host="${url#*://}"
  host="${host%%/*}"
  host="${host%%:*}"
  printf '%s\n' "$host"
}

wait_for_transparent_log() {
  local host i log_file
  host="$(url_host "$TEST_URL")"
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] && grep -q -- "--> $host:443" "$log_file"; then
      echo "transparent TUN log observed for $host:443"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log transparent TUN traffic for $host:443" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
}

wait_for_tun_policy_log() {
  local group policy host i log_file
  group="$1"
  policy="$2"
  host="$(url_host "$TEST_URL")"
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "--> $host:443" "$log_file" | grep -Fq -- "using $group[$policy]"; then
      echo "transparent TUN policy log observed for $host:443 using $group[$policy]"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log transparent TUN traffic for $host:443 using $group[$policy]" >&2
  tail -100 "$log_file" >&2 || true
  exit 1
}

wait_for_tun_policy_log_for_host() {
  local group policy host i log_file
  group="$1"
  policy="$2"
  host="$3"
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "--> $host:443" "$log_file" | grep -Fq -- "using $group[$policy]"; then
      echo "device TUN policy log observed for $host:443 using $group[$policy]"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log device TUN traffic for $host:443 using $group[$policy]" >&2
  tail -120 "$log_file" >&2 || true
  exit 1
}

wait_for_tun_action_log() {
  local host action i log_file
  host="$1"
  action="$2"
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "--> $host:443" "$log_file" | grep -Fq -- "using $action"; then
      echo "device TUN action log observed for $host:443 using $action"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log device TUN traffic for $host:443 using $action" >&2
  tail -120 "$log_file" >&2 || true
  exit 1
}

wait_for_policy_option() {
  local group option output error i
  group="$1"
  option="$2"
  output="$STATE_DIR/policies-wait.json"
  error="$STATE_DIR/policies-wait.err"
  for i in {1..50}; do
    if "$BINARY" policies --config "$CONFIG" --format json >"$output" 2>"$error" &&
      grep -Fq -- "\"name\": \"$group\"" "$output" &&
      grep -Fq -- "\"$option\"" "$output"; then
      return 0
    fi
    sleep 0.2
  done
  echo "policy group $group did not include option $option" >&2
  cat "$output" >&2 || true
  cat "$error" >&2 || true
  tail -120 "$STATE_DIR/logs/mihomo.log" >&2 || true
  exit 1
}

build_egress_probe() {
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$EGRESS_PROBE_BINARY" ./tests/integration/egressprobe
}

start_egress_probe() {
  local log_file i
  mkdir -p "$STATE_DIR/logs"
  rm -rf "$STATE_DIR/egress"
  log_file="$STATE_DIR/logs/egress-probe.log"
  "$EGRESS_PROBE_BINARY" \
    --origin "127.0.0.1:$EGRESS_ORIGIN_PORT" \
    --proxy "127.0.0.1:$EGRESS_PROXY_PORT" \
    --provider-file "$EGRESS_PROVIDER" \
    --provider-path "/tun-egress-provider.yaml" \
    --log-dir "$STATE_DIR/egress" >"$log_file" 2>&1 &
  EGRESS_PROBE_PID=$!
  for i in {1..50}; do
    if grep -Fq READY "$log_file" 2>/dev/null; then
      echo "TUN egress probe ready: provider=$EGRESS_PROVIDER_URL proxy=127.0.0.1:$EGRESS_PROXY_PORT"
      return 0
    fi
    if ! kill -0 "$EGRESS_PROBE_PID" 2>/dev/null; then
      echo "TUN egress probe exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
  echo "TUN egress probe did not become ready" >&2
  cat "$log_file" >&2 || true
  exit 1
}

stop_egress_probe() {
  if [[ -n "$EGRESS_PROBE_PID" ]] && kill -0 "$EGRESS_PROBE_PID" 2>/dev/null; then
    kill "$EGRESS_PROBE_PID" 2>/dev/null || true
    wait "$EGRESS_PROBE_PID" 2>/dev/null || true
  fi
  EGRESS_PROBE_PID=""
}

assert_tun_egress_proxy_unused() {
  if [[ -s "$STATE_DIR/egress/proxy.log" ]]; then
    echo "TunEgress DIRECT unexpectedly used the controlled proxy" >&2
    cat "$STATE_DIR/egress/proxy.log" >&2
    exit 1
  fi
}

assert_tun_egress_proxy_used() {
  local host
  host="$(url_host "$TEST_URL")"
  if ! grep -Fq -- "CONNECT $host:443" "$STATE_DIR/egress/proxy.log" 2>/dev/null; then
    echo "controlled proxy did not observe CONNECT $host:443" >&2
    cat "$STATE_DIR/egress/proxy.log" >&2 || true
    exit 1
  fi
}

client_mac() {
  local client=$1
  limactl shell "$client" -- cat /sys/class/net/omg0/address | tr -d '\r\n'
}

assert_client_ipv4() {
  local client=$1 expected=$2 actual
  actual="$(limactl shell "$client" -- bash -lc "ip -4 -o addr show dev omg0 scope global | awk 'NR == 1 { split(\$4, value, \"/\"); print value[1] }'" | tr -d '\r\n')"
  if [[ "$actual" != "$expected" ]]; then
    echo "$client IPv4 $actual, want DHCP reservation $expected" >&2
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status >&2 || true
    exit 1
  fi
}

write_device_policy_fixture() {
  local client_one client_two mac_one mac_two
  set -- $CLIENTS
  if [[ "$#" -ne 2 ]]; then
    echo "device-policy lab requires exactly two clients; set OMG_LAB_CLIENTS to two names" >&2
    exit 1
  fi
  client_one="$1"
  client_two="$2"
  mac_one="$(client_mac "$client_one")"
  mac_two="$(client_mac "$client_two")"
  [[ -n "$mac_one" && -n "$mac_two" && "$mac_one" != "$mac_two" ]] || {
    echo "device-policy lab could not resolve two distinct client MAC addresses" >&2
    exit 1
  }

  LAB_DEVICE_POLICY_FILE="$STATE_DIR/device-policy.json"
  LAB_MIHOMO_PROFILE="$STATE_DIR/mihomo-profile.device-policy.yaml"
  cat >"$LAB_MIHOMO_PROFILE" <<EOF
proxies:
  - name: lab-controlled
    type: http
    server: 127.0.0.1
    port: $EGRESS_PROXY_PORT
rules:
  - MATCH,DIRECT
EOF
  cat >"$LAB_DEVICE_POLICY_FILE" <<EOF
{
  "profiles": [
    {
      "id": "controlled",
      "default_policies": ["lab-controlled", "DIRECT"]
    },
    {
      "id": "direct-blocked",
      "default_policies": ["DIRECT", "lab-controlled"]
    }
  ],
  "devices": [
    {
      "id": "$client_one",
      "mac": "$mac_one",
      "ipv4": "192.168.50.101",
      "profile": "controlled"
    },
    {
      "id": "$client_two",
      "mac": "$mac_two",
      "ipv4": "192.168.50.102",
      "profile": "direct-blocked"
    }
  ]
}
EOF
}

write_device_block_rule() {
  local client_one=$1 client_two=$2 mac_one mac_two host=$3
  mac_one="$(client_mac "$client_one")"
  mac_two="$(client_mac "$client_two")"
  cat >"$LAB_DEVICE_POLICY_FILE" <<EOF
{
  "profiles": [
    {
      "id": "controlled",
      "default_policies": ["lab-controlled", "DIRECT"]
    },
    {
      "id": "direct-blocked",
      "default_policies": ["DIRECT", "lab-controlled"],
      "rules": [
        {
          "id": "block-test-host",
          "match": {"domains": ["$host"]},
          "action": "REJECT"
        }
      ]
    }
  ],
  "devices": [
    {
      "id": "$client_one",
      "mac": "$mac_one",
      "ipv4": "192.168.50.101",
      "profile": "controlled"
    },
    {
      "id": "$client_two",
      "mac": "$mac_two",
      "ipv4": "192.168.50.102",
      "profile": "direct-blocked"
    }
  ]
}
EOF
}

run_device_policy_test() {
  local client_one client_two gateway_started egress_probe_started host
  require_installed_lab
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_cached_sudo
  ensure_lab_state_writable
  set -- $CLIENTS
  [[ "$#" -eq 2 ]] || { echo "device-policy lab requires exactly two clients" >&2; exit 1; }
  client_one="$1"
  client_two="$2"
  host="$(url_host "$TEST_URL")"

  mkdir -p "$STATE_DIR"
  rm -f "$STATE_DIR/cache.db" "$STATE_DIR/cache.db-journal"
  write_device_policy_fixture
  write_config tun
  require_command go
  mkdir -p "$ROOT/bin"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$BINARY" ./cmd/omg
  build_egress_probe

  gateway_started=0
  egress_probe_started=0
  cleanup_test() {
    status=$?
    collect_artifacts || true
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    if [[ "$egress_probe_started" == 1 ]]; then
      stop_egress_probe || true
    fi
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  start_egress_probe
  egress_probe_started=1
  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  assert_client_ipv4 "$client_one" "192.168.50.101"
  assert_client_ipv4 "$client_two" "192.168.50.102"
  "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/device-policies.json"
  grep -Fq '"ipv4": "192.168.50.101"' "$STATE_DIR/device-policies.json"
  grep -Fq '"ipv4": "192.168.50.102"' "$STATE_DIR/device-policies.json"

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "lab-controlled" "$host"
  assert_tun_egress_proxy_used

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_two/default" "DIRECT" "$host"
  assert_tun_egress_proxy_unused

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_one" --slot default --policy DIRECT --format json >"$STATE_DIR/device-one-direct.json"
  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "DIRECT" "$host"
  assert_tun_egress_proxy_unused

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy lab-controlled --format json >"$STATE_DIR/device-two-controlled.json"
  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_two/default" "lab-controlled" "$host"
  assert_tun_egress_proxy_used
  "$BINARY" policies --config "$CONFIG" --format json >"$STATE_DIR/device-policies-live.json"
  grep -A 4 -F "\"name\": \"device/$client_one/default\"" "$STATE_DIR/device-policies-live.json" | grep -Fq '"selected": "DIRECT"'
  grep -A 4 -F "\"name\": \"device/$client_two/default\"" "$STATE_DIR/device-policies-live.json" | grep -Fq '"selected": "lab-controlled"'

  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  write_device_block_rule "$client_one" "$client_two" "$host"
  write_config tun
  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1

  : >"$STATE_DIR/egress/proxy.log"
  if limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"; then
    echo "device-specific REJECT unexpectedly allowed $host" >&2
    exit 1
  fi
  wait_for_tun_action_log "$host" "REJECT"
  assert_tun_egress_proxy_unused

  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  stop_egress_probe
  egress_probe_started=0
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  trap - EXIT INT TERM
  collect_artifacts
  echo "virtual LAN device-policy TUN test passed"
}

run_test() {
  local mode client gateway_started egress_probe_started
  mode="${1:-off}"
  require_installed_lab
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_cached_sudo
  ensure_lab_state_writable
  write_config "$mode"
  require_command go
  mkdir -p "$ROOT/bin"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$BINARY" ./cmd/omg

  gateway_started=0
  egress_probe_started=0
  cleanup_test() {
    status=$?
    collect_artifacts || true
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    if [[ "$egress_probe_started" == 1 ]]; then
      stop_egress_probe || true
    fi
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  if [[ "$TUN_EGRESS_PROFILE" == 1 ]]; then
    build_egress_probe
    start_egress_probe
    egress_probe_started=1
  fi

  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1

  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  done

  if [[ "$mode" == "tun" && "$TUN_EGRESS_PROFILE" == 1 ]]; then
    wait_for_policy_option TunEgress egress-proxy
    "$BINARY" providers --config "$CONFIG" --format json >"$STATE_DIR/tun-egress-providers.json"
    grep -Fq '"name": "tun-egress-provider"' "$STATE_DIR/tun-egress-providers.json"
    grep -Fq '"name": "egress-proxy"' "$STATE_DIR/tun-egress-providers.json"
    for client in $CLIENTS; do
      limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
    done
    wait_for_tun_policy_log TunEgress DIRECT
    assert_tun_egress_proxy_unused

    "$BINARY" policy-select --config "$CONFIG" --group TunEgress --policy egress-proxy --format json
    for client in $CLIENTS; do
      limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
    done
    wait_for_tun_policy_log TunEgress egress-proxy
    assert_tun_egress_proxy_used
  else
    for client in $CLIENTS; do
      if [[ "$mode" == "tun" ]]; then
        limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
      else
        limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client test "$LAN_IP" "$TEST_URL"
      fi
    done
    if [[ "$mode" == "tun" ]]; then
      wait_for_transparent_log
    fi
  fi

  "$BINARY" status --config "$CONFIG"
  "$BINARY" leases --config "$CONFIG"
  lease_count=$(awk 'NF >= 4 { count++ } END { print count + 0 }' "$STATE_DIR/dnsmasq.leases")
  expected_count=$(wc -w <<<"$CLIENTS" | tr -d ' ')
  if ((lease_count < expected_count)); then
    echo "expected at least $expected_count DHCP leases, got $lease_count" >&2
    exit 1
  fi

  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  if [[ "$egress_probe_started" == 1 ]]; then
    stop_egress_probe
    egress_probe_started=0
  fi
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  trap - EXIT INT TERM
  collect_artifacts
  echo "virtual LAN ${mode} test passed"
}

check_lab() {
  require_installed_lab
  limactl --version
  /opt/socket_vmnet/bin/socket_vmnet --version
  dnsmasq --version | head -1
  mihomo -v | head -1
  sudo -n "$NETWORK_HELPER" status || true
}

case "${1:-}" in
  check)
    check_lab
    ;;
  up)
    require_installed_lab
    start_network
    write_config off
    start_clients
    echo "Lab ready: interface=$(lab_interface) config=$CONFIG clients=$CLIENTS"
    ;;
  status)
    require_installed_lab
    sudo -n "$NETWORK_HELPER" status || true
    if [[ -f "$CONFIG" ]]; then
      echo "config=$CONFIG"
      "$BINARY" status --config "$CONFIG" 2>/dev/null || true
    fi
    for client in $CLIENTS; do
      if [[ -d "$(instance_dir "$client")" ]]; then
        client_state="$(limactl list --format '{{.Status}}' "$client" 2>/dev/null || true)"
        if [[ "$client_state" == "Running" ]]; then
          limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status || true
        else
          echo "$client: ${client_state:-unknown}"
        fi
      fi
    done
    ;;
  test)
    run_test off
    ;;
  test-tun)
    run_test tun
    ;;
  test-tun-device-policy)
    run_device_policy_test
    ;;
  down)
    stop_clients
    if [[ -x "$NETWORK_HELPER" ]]; then
      sudo -n "$NETWORK_HELPER" stop
    fi
    ;;
  destroy)
    destroy_clients
    if [[ -x "$NETWORK_HELPER" ]]; then
      sudo -n "$NETWORK_HELPER" stop
    fi
    ;;
  *)
    echo "usage: $0 <check|up|status|test|test-tun|test-tun-device-policy|down|destroy>" >&2
    exit 2
    ;;
esac
