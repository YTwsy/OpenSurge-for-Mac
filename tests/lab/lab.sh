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
LAN_IP=192.168.50.1
CLIENTS="${OMG_LAB_CLIENTS:-omg-lab-client-1 omg-lab-client-2}"
TEST_URL="${OMG_LAB_TEST_URL:-https://example.com/}"

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
  for name in HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy FTP_PROXY ftp_proxy grpc_proxy NO_PROXY no_proxy; do
    value="${!name:-}"
    if [[ -n "$value" ]]; then
      printf '%sexport %s=%s\n' "$indent" "$name" "$(shell_quote "$value")"
    fi
  done
}

write_config() {
	local iface upstream dnsmasq_bin mihomo_bin runtime_dir
	iface="$(lab_interface)"
	upstream="$(upstream_interface)"
	dnsmasq_bin="$(command -v dnsmasq)"
	mihomo_bin="$(command -v mihomo)"
  runtime_dir="$STATE_DIR"

  case "$iface" in
    bridge*) ;;
    *) echo "refusing non-bridge lab interface: $iface" >&2; exit 1 ;;
  esac
  /sbin/ifconfig "$iface" | grep -q 'member: vmenet'
  /sbin/ifconfig "$iface" | grep -q "inet $LAN_IP "
  [[ "$iface" != "$upstream" ]] || { echo "lab and upstream interfaces must differ" >&2; exit 1; }

  mkdir -p "$STATE_DIR"
  sed \
	  -e "s|__LAB_INTERFACE__|$(sed_escape "$iface")|g" \
	  -e "s|__UPSTREAM_INTERFACE__|$(sed_escape "$upstream")|g" \
	  -e "s|__DNSMASQ_BINARY__|$(sed_escape "$dnsmasq_bin")|g" \
	  -e "s|__MIHOMO_BINARY__|$(sed_escape "$mihomo_bin")|g" \
    -e "s|__RUNTIME_DIR__|$(sed_escape "$runtime_dir")|g" \
    "$CONFIG_TEMPLATE" >"$CONFIG"
}

write_client_config() {
  local image line
  image="$TOOLS_ROOT/cache/nocloud_alpine-3.23.4-aarch64-uefi-cloudinit-r0.qcow2"
  mkdir -p "$STATE_DIR"
  : >"$CLIENT_CONFIG"
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      *__PROXY_EXPORTS__*)
        write_proxy_exports "      "
        ;;
      *)
        printf '%s\n' "${line//__ALPINE_IMAGE__/$image}"
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
      limactl create --name "$client" "$CLIENT_CONFIG"
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

run_test() {
  local client gateway_started
  require_installed_lab
  [[ -f "$CONFIG" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_command go
  mkdir -p "$ROOT/bin"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$BINARY" ./cmd/omg

  gateway_started=0
  cleanup_test() {
    status=$?
    collect_artifacts || true
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  if ! sudo -n "$BINARY" start --config "$CONFIG"; then
    echo "gateway test requires a cached sudo credential; run 'sudo -v' in a terminal, then retry" >&2
    exit 1
  fi
  gateway_started=1

  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client test "$LAN_IP" "$TEST_URL"
  done

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
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  trap - EXIT INT TERM
  collect_artifacts
  echo "virtual LAN test passed"
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
    write_config
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
        limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status || true
      fi
    done
    ;;
  test)
    run_test
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
    echo "usage: $0 <check|up|status|test|down|destroy>" >&2
    exit 2
    ;;
esac
