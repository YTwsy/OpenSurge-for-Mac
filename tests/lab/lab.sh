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
CONTROL_API_BINARY="$STATE_DIR/opensurge-control"
EGRESS_PROVIDER="$STATE_DIR/tun-egress-provider.yaml"
IPV6_PACKET_BINARY="$STATE_DIR/opensurge-network"
PATCHED_MIHOMO_BINARY="$STATE_DIR/mihomo-opensurge"
HTTP3_PROBE_BINARY="$STATE_DIR/opensurge-http3-probe"
HTTP3_CLIENT_BINARY="$STATE_DIR/opensurge-http3-probe-linux-arm64"
HTTP3_CLIENT_GUEST=/usr/local/bin/opensurge-http3-probe
EGRESS_ORIGIN_PORT="${OMG_LAB_TUN_EGRESS_ORIGIN_PORT:-19093}"
EGRESS_PROXY_PORT="${OMG_LAB_TUN_EGRESS_PROXY_PORT:-19094}"
EGRESS_PROVIDER_URL="http://127.0.0.1:$EGRESS_ORIGIN_PORT/tun-egress-provider.yaml"
IPV6_DNS_FIXTURE_PORT="${OMG_LAB_IPV6_DNS_FIXTURE_PORT:-19095}"
IPV6_HTTP3_FIXTURE_PORT="${OMG_LAB_IPV6_HTTP3_FIXTURE_PORT:-19096}"
IPV6_UDP_PROXY_PORT="${OMG_LAB_IPV6_UDP_PROXY_PORT:-19097}"
IPV6_UDP_PROXY_DNS_PORT="${OMG_LAB_IPV6_UDP_PROXY_DNS_PORT:-19098}"
CONTROL_API_PORT="${OMG_LAB_CONTROL_API_PORT:-19099}"
CONFIG_DNS_PORT=1053
CONNECTION_REFRESH_TEST_HOST="connection-refresh.opensurge.test"
IPV6_TCP_TEST_HOST="ipv6-tcp.opensurge.test"
IPV6_TCP_TEST_URL="http://$IPV6_TCP_TEST_HOST:$EGRESS_ORIGIN_PORT/ipv6-tcp"
IPV6_UDP_TEST_HOST="ipv6-udp.opensurge.test"
IPV6_UDP_ANSWER_HOST="udp-answer.opensurge.test"
IPV6_QUIC_TEST_HOST="ipv6-quic.opensurge.test"
IPV6_HTTP3_DIRECT_HOST="ipv6-http3-direct.opensurge.test"
IPV6_HTTP3_PROXY_HOST="ipv6-http3-proxy.opensurge.test"
IPV6_HTTP3_BLOCKED_HOST="ipv6-http3-blocked.opensurge.test"
LOCAL_ROUTING_IPV6_HTTP3_HOST="local-ipv6-http3.opensurge.test"
IPV6_REAL_PROFILE_SOURCE="${OMG_LAB_IPV6_REAL_PROFILE:-}"
IPV6_REAL_PROFILE="$STATE_DIR/mihomo-profile.ipv6-real.yaml"
IPV6_REAL_TCP_HOST="${OMG_LAB_IPV6_REAL_TCP_HOST:-api64.ipify.org}"
IPV6_REAL_TCP_URL="${OMG_LAB_IPV6_REAL_TCP_URL:-https://api64.ipify.org/}"
IPV6_REAL_TARGET="${OMG_LAB_IPV6_REAL_TARGET:-2606:4700:4700::1111}"
IPV6_REAL_TARGET_URL="${OMG_LAB_IPV6_REAL_TARGET_URL:-https://[2606:4700:4700::1111]/cdn-cgi/trace}"
IPV6_REAL_UDP_TARGET="${OMG_LAB_IPV6_REAL_UDP_TARGET:-2001:4860:4860::8888}"
IPV6_REAL_UDP_QUERY="${OMG_LAB_IPV6_REAL_UDP_QUERY:-cloudflare.com}"
IPV6_NATIVE_DIRECT_HOST="${OMG_LAB_IPV6_NATIVE_DIRECT_HOST:-api6.ipify.org}"
IPV6_NATIVE_DIRECT_URL="${OMG_LAB_IPV6_NATIVE_DIRECT_URL:-https://api6.ipify.org/}"
IPV6_NATIVE_DIRECT_TARGET="${OMG_LAB_IPV6_NATIVE_DIRECT_TARGET:-2001:4860:4860::8888}"
LAN_IP=192.168.50.1
SAME_WIFI_BYPASS_GATEWAY=192.168.49.254
CLIENTS="${OMG_LAB_CLIENTS:-omg-lab-client-1 omg-lab-client-2}"
TEST_URL="${OMG_LAB_TEST_URL:-https://example.com/}"
LAB_MIHOMO_PROFILE="${OMG_LAB_MIHOMO_PROFILE:-}"
LAB_DEVICE_POLICY_FILE=""
TUN_EGRESS_PROFILE=0
LOCAL_ROUTING_TEST="${OMG_LAB_LOCAL_ROUTING_TEST:-false}"
EGRESS_PROBE_PID=""
CONTROL_API_PID=""
CONTROL_API_TOKEN=""
LAST_CLIENT_HOLD_PID=""
IPV6_DNS_FIXTURE_PID=""
HTTP3_PROBE_PID=""
IPV6_UDP_PROXY_PID=""
LAST_LAB_ARTIFACT_DIR=""
REAL_PROXY_TYPE=""
REAL_PROXY_INDEX=""
REAL_PROXY_EXIT_FAMILY=""
SUDO_KEEPALIVE_PID=""

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
  if sudo -n true 2>/dev/null; then
    return 0
  fi
  if [[ -n "${OMG_LAB_SUDO_ASKPASS:-}" && -x "${OMG_LAB_SUDO_ASKPASS}" ]]; then
    SUDO_ASKPASS="$OMG_LAB_SUDO_ASKPASS" sudo -A -v 2>/dev/null || true
    if sudo -n true 2>/dev/null; then
      return 0
    fi
  fi
  echo "gateway test requires a cached sudo credential; run 'sudo -v' in a terminal, then retry" >&2
  exit 1
}

start_sudo_keepalive() {
  [[ -z "$SUDO_KEEPALIVE_PID" ]] || return 0
  (
    while sudo -n -v 2>/dev/null; do
      sleep 15
    done
  ) &
  SUDO_KEEPALIVE_PID=$!
}

stop_sudo_keepalive() {
  [[ -n "$SUDO_KEEPALIVE_PID" ]] || return 0
  /bin/kill "$SUDO_KEEPALIVE_PID" 2>/dev/null || true
  wait "$SUDO_KEEPALIVE_PID" 2>/dev/null || true
  SUDO_KEEPALIVE_PID=""
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
  local source=$1 destination=$2 host resolver
  host="$(url_host "$TEST_URL")"
  resolver="1.1.1.1"
  if [[ "$LOCAL_ROUTING_TEST" == "true" ]]; then
    resolver="127.0.0.1:$IPV6_DNS_FIXTURE_PORT"
  fi
  write_tun_egress_provider
  sed \
    -e "s|__TUN_EGRESS_PROVIDER_URL__|$(sed_escape "$EGRESS_PROVIDER_URL")|g" \
    -e "s|__TUN_EGRESS_HOST__|$(sed_escape "$host")|g" \
    -e "s|__TUN_EGRESS_DNS__|$(sed_escape "$resolver")|g" \
    -e "s|__LOCAL_ROUTING_IPV6_HOST__|$(sed_escape "$LOCAL_ROUTING_IPV6_HTTP3_HOST")|g" \
    "$source" >"$destination"
}

write_config() {
  local mode iface upstream dnsmasq_bin mihomo_bin runtime_dir dns_upstream_line device_policy_file
  local transparent_mode dns_ipv6 tun_ipv6 ipv6_packet_binary gateway_mode dhcp_enabled ipv6_shared_l2_ready
  local profile_mode profile_path profile_source dhcp_bypass_gateway dhcp_bypass_dns
	mode="${1:-off}"
  TUN_EGRESS_PROFILE=0
	iface="$(lab_interface)"
	upstream="$(upstream_interface)"
	dnsmasq_bin="$(command -v dnsmasq)"
	mihomo_bin="${OMG_LAB_MIHOMO_BINARY:-$(command -v mihomo)}"
  [[ -x "$mihomo_bin" ]] || { echo "mihomo binary is not executable: $mihomo_bin" >&2; exit 1; }
  runtime_dir="$STATE_DIR"
  device_policy_file="$LAB_DEVICE_POLICY_FILE"
  dns_upstream_line=""
  dns_ipv6=false
  tun_ipv6=off
  gateway_mode=isolated_lan
  dhcp_enabled=true
  dhcp_bypass_gateway=""
  dhcp_bypass_dns=""
  ipv6_shared_l2_ready=false
  ipv6_packet_binary=opensurge-network
  transparent_mode="$mode"
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
    ipv6|ipv6-auto|ipv6-same-wifi|ipv6-same-lan)
      transparent_mode=tun
      dns_upstream_line='  upstream: "127.0.0.1#1053"'
      dns_ipv6=true
      if [[ "$mode" == "ipv6-auto" ]]; then
        tun_ipv6=auto
      else
        tun_ipv6=always
      fi
      ipv6_packet_binary="$IPV6_PACKET_BINARY"
      mihomo_bin="$PATCHED_MIHOMO_BINARY"
      if [[ "$mode" == "ipv6-same-wifi" ]]; then
        gateway_mode=same_wifi_dhcp
        upstream="$iface"
        ipv6_shared_l2_ready=true
        # Use an address outside the dynamic pool as the fixture's stand-in
        # upstream router. IPv6 still enters through the OpenSurge packet path.
        dhcp_bypass_gateway="$SAME_WIFI_BYPASS_GATEWAY"
        dhcp_bypass_dns=192.168.50.1
      elif [[ "$mode" == "ipv6-same-lan" ]]; then
        gateway_mode=same_lan
        upstream="$iface"
        dhcp_enabled=false
        ipv6_shared_l2_ready=true
      fi
      ;;
    *) echo "unknown transparent mode: $mode" >&2; exit 2 ;;
  esac
  if [[ "$LOCAL_ROUTING_TEST" == "true" ]]; then
    dns_ipv6=true
  fi

  case "$iface" in
    bridge*) ;;
    *) echo "refusing non-bridge lab interface: $iface" >&2; exit 1 ;;
  esac
  /sbin/ifconfig "$iface" | grep -q 'member: vmenet'
  /sbin/ifconfig "$iface" | grep -q "inet $LAN_IP "
  require_unique_lab_ip "$iface"
  if [[ "$gateway_mode" == "isolated_lan" ]]; then
    [[ "$iface" != "$upstream" ]] || { echo "isolated lab and upstream interfaces must differ" >&2; exit 1; }
  else
    [[ "$iface" == "$upstream" ]] || { echo "shared-L2 lab requires matching gateway and upstream interfaces" >&2; exit 1; }
  fi

  mkdir -p "$STATE_DIR"
  sed \
	  -e "s|__LAB_INTERFACE__|$(sed_escape "$iface")|g" \
	  -e "s|__UPSTREAM_INTERFACE__|$(sed_escape "$upstream")|g" \
	  -e "s|__GATEWAY_MODE__|$(sed_escape "$gateway_mode")|g" \
	  -e "s|__DHCP_ENABLED__|$(sed_escape "$dhcp_enabled")|g" \
	  -e "s|__DHCP_BYPASS_GATEWAY__|$(sed_escape "$dhcp_bypass_gateway")|g" \
	  -e "s|__DHCP_BYPASS_DNS__|$(sed_escape "$dhcp_bypass_dns")|g" \
	  -e "s|__DNSMASQ_BINARY__|$(sed_escape "$dnsmasq_bin")|g" \
	  -e "s|__MIHOMO_BINARY__|$(sed_escape "$mihomo_bin")|g" \
    -e "s|__MIHOMO_PROFILE_MODE__|$(sed_escape "$profile_mode")|g" \
    -e "s|__MIHOMO_PROFILE__|$(sed_escape "$profile_path")|g" \
    -e "s|__DEVICE_POLICY_FILE__|$(sed_escape "$device_policy_file")|g" \
    -e "s|__DNS_UPSTREAM_LINE__|$(sed_escape "$dns_upstream_line")|g" \
    -e "s|__DNS_IPV6__|$(sed_escape "$dns_ipv6")|g" \
    -e "s|__TUN_IPV6__|$(sed_escape "$tun_ipv6")|g" \
    -e "s|__IPV6_SHARED_L2_READY__|$(sed_escape "$ipv6_shared_l2_ready")|g" \
    -e "s|__IPV6_PACKET_BROKER_BINARY__|$(sed_escape "$ipv6_packet_binary")|g" \
    -e "s|__TRANSPARENT_MODE__|$(sed_escape "$transparent_mode")|g" \
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
  local client instance_yaml pid failed cold_start
  local -a start_pids
  write_client_config
  cold_start=0
  start_pids=()
  for client in $CLIENTS; do
    instance_yaml="$(instance_dir "$client")/lima.yaml"
    if [[ -f "$instance_yaml" ]] && ! cmp -s "$instance_yaml" "$CLIENT_CONFIG"; then
      limactl stop "$client" || true
      limactl delete -f -y "$client"
      cold_start=1
    fi
    if [[ ! -d "$(instance_dir "$client")" ]]; then
      limactl create -y --name "$client" "$CLIENT_CONFIG"
      cold_start=1
    fi
  done

  # Keep cold provisioning sequential so two apt jobs cannot contend for the
  # same upstream bandwidth and push each VM past Lima's readiness timeout.
  # Stable, already-provisioned clients have no apt work and can boot in
  # parallel, which avoids adding two independent guest boot times together.
  if [[ "$cold_start" == 1 ]]; then
    for client in $CLIENTS; do
      limactl start "$client"
      limactl shell "$client" -- true
    done
    return 0
  fi

  for client in $CLIENTS; do
    limactl start "$client" &
    start_pids+=("$!")
  done
  failed=0
  for pid in "${start_pids[@]}"; do
    wait "$pid" || failed=1
  done
  if [[ "$failed" != 0 ]]; then
    echo "one or more persistent lab clients failed to start" >&2
    return 1
  fi
  for client in $CLIENTS; do
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

restore_client_control_dns() {
  local client client_state
  for client in $CLIENTS; do
    [[ -d "$(instance_dir "$client")" ]] || continue
    client_state="$(limactl list --format '{{.Status}}' "$client" 2>/dev/null || true)"
    [[ "$client_state" == "Running" ]] || continue
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client restore-control || true
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
  local artifact_dir client evidence
  artifact_dir="$ROOT/artifacts/lab/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$artifact_dir"
  cp "$CONFIG" "$artifact_dir/config.yaml" 2>/dev/null || true
  cp "$LAB_DEVICE_POLICY_FILE" "$artifact_dir/device-policy.json" 2>/dev/null || true
  cp "$LAB_MIHOMO_PROFILE" "$artifact_dir/device-policy-profile.yaml" 2>/dev/null || true
  cp "$EGRESS_PROVIDER" "$artifact_dir/tun-egress-provider.yaml" 2>/dev/null || true
  cp -R "$STATE_DIR/egress" "$artifact_dir/egress" 2>/dev/null || true
  cp -R "$STATE_DIR/logs" "$artifact_dir/logs" 2>/dev/null || true
  for evidence in \
    dnsmasq.conf \
    mihomo.yaml \
    device-policy.applied.evidence.json \
    state.evidence.json \
    device-policies.json \
    device-policies-after-reload.json \
    connection-refresh-before.json \
    connection-refresh-after.json \
    connection-refresh-response.json \
    ipv6-status.json \
    ipv6-devices.json \
    ipv6-state.evidence.json; do
    cp "$STATE_DIR/$evidence" "$artifact_dir/$evidence" 2>/dev/null || true
  done
  "$BINARY" status --config "$CONFIG" >"$artifact_dir/gateway-status.txt" 2>&1 || true
  "$BINARY" leases --config "$CONFIG" >"$artifact_dir/leases.txt" 2>&1 || true
  /sbin/ifconfig "$(lab_interface)" >"$artifact_dir/interface.txt" 2>&1 || true
  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status \
      >"$artifact_dir/$client.txt" 2>&1 || true
  done
  echo "Lab artifacts: $artifact_dir"
}

collect_ipv6_real_artifacts() {
  local artifact_dir client evidence
  artifact_dir="$ROOT/artifacts/lab/$(date +%Y%m%d-%H%M%S)-ipv6-real"
  mkdir -p "$artifact_dir"
  for evidence in ipv6-real-status.json ipv6-real-egress.txt dnsmasq.conf; do
    cp "$STATE_DIR/$evidence" "$artifact_dir/$evidence" 2>/dev/null || true
  done
  "$BINARY" status --config "$CONFIG" >"$artifact_dir/gateway-status.txt" 2>&1 || true
  /sbin/ifconfig "$(lab_interface)" >"$artifact_dir/interface.txt" 2>&1 || true
  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status \
      >"$artifact_dir/$client.txt" 2>&1 || true
  done
  LAST_LAB_ARTIFACT_DIR="$artifact_dir"
  echo "Secret-safe IPv6 real-egress artifacts: $artifact_dir"
}

assert_ipv6_real_artifacts_safe() {
  local artifact_dir=$1 profile=$2
  case "$artifact_dir" in
    "$ROOT"/artifacts/lab/*-ipv6-real) ;;
    *) echo "refusing to inspect unexpected artifact directory: $artifact_dir" >&2; return 1 ;;
  esac
  for forbidden in config.yaml mihomo.yaml device-policy-profile.yaml cache.db mihomo.log; do
    if find "$artifact_dir" -name "$forbidden" -print -quit | grep -q .; then
      echo "secret-bearing file was copied into IPv6 real-egress artifacts: $forbidden" >&2
      return 1
    fi
  done
  /usr/bin/ruby -ryaml -rdate -e '
    profile_path, artifact_dir = ARGV
    profile = YAML.safe_load(File.read(profile_path), permitted_classes: [Date, Time], aliases: true)
    proxies = profile.is_a?(Hash) ? Array(profile["proxies"]) : []
    markers = proxies.flat_map do |proxy|
      next [] unless proxy.is_a?(Hash)
      values = %w[server password uuid token].map { |key| proxy[key].to_s unless proxy[key].to_s.empty? }.compact
      name = proxy["name"].to_s
      values << name if name.bytesize >= 12
      values
    end.uniq.select { |value| value.bytesize >= 4 }
    Dir.glob(File.join(artifact_dir, "**", "*"), File::FNM_DOTMATCH).each do |path|
      next unless File.file?(path)
      body = File.binread(path)
      if markers.any? { |marker| body.include?(marker.b) }
        warn "subscription marker found in secret-safe artifact #{File.basename(path)}"
        exit 1
      end
    end
  ' "$profile" "$artifact_dir"
}

cleanup_ipv6_real_secrets() {
  rm -f "$IPV6_REAL_PROFILE" \
    "$STATE_DIR/mihomo.yaml" \
    "$STATE_DIR/cache.db" \
    "$STATE_DIR/cache.db-journal" \
    "$STATE_DIR/logs/mihomo.log"
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
  local host action source_ip i log_file
  host="$1"
  action="$2"
  source_ip="${3:-}"
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "--> $host:443" "$log_file" | grep -F -- "$source_ip" | grep -Fq -- "using $action"; then
      echo "device TUN action log observed for $host:443 using $action"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log device TUN traffic for $host:443 using $action" >&2
  tail -120 "$log_file" >&2 || true
  exit 1
}

wait_for_tun_source_log_after() {
  local host=$1 source_ip=$2 first_line=$3 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      tail -n +"$first_line" "$log_file" | grep -F -- "--> $host:443" | grep -Fq -- "$source_ip"; then
      echo "TUN source log observed for $source_ip -> $host:443"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log TUN traffic for $source_ip -> $host:443 after line $first_line" >&2
  tail -160 "$log_file" >&2 || true
  exit 1
}

wait_for_local_ipv6_log_after() {
  local network=$1 source_ip=$2 host=$3 port=$4 action=$5 first_line=$6 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..30}; do
    if [[ -f "$log_file" ]] &&
      tail -n +"$first_line" "$log_file" | grep -F "[$network]" | grep -F -- "$source_ip" |
        grep -F -- "--> $host:$port" | grep -Fq -e "using $action" -e "dial $action "; then
      echo "local IPv6 $network log observed for $source_ip -> $host:$port via $action"
      return 0
    fi
    sleep 0.2
  done
  echo "mihomo did not log local IPv6 $network traffic for $source_ip -> $host:$port via $action" >&2
  tail -180 "$log_file" >&2 || true
  exit 1
}

wait_for_tun_udp_reject() {
  local source_ip=$1 target=$2 port=$3 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..20}; do
    if [[ -f "$log_file" ]] &&
      grep -E "\\[UDP\\].*$source_ip.*--> $target:$port.*using REJECT" "$log_file" >/dev/null; then
      if grep -E "\\[UDP\\].*$source_ip.*--> $target:$port.*using DIRECT" "$log_file" >/dev/null; then
        echo "device UDP probe reached DIRECT instead of failing closed" >&2
        tail -160 "$log_file" >&2 || true
        exit 1
      fi
      echo "device UDP fail-closed log observed for $source_ip -> $target:$port"
      return 0
    fi
    sleep 1
  done
  echo "mihomo did not log device UDP REJECT for $source_ip -> $target:$port" >&2
  tail -160 "$log_file" >&2 || true
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

build_control_api() {
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$CONTROL_API_BINARY" ./cmd/opensurge-control
}

start_egress_probe() {
  local log_file i
  mkdir -p "$STATE_DIR/logs"
  rm -rf "$STATE_DIR/egress"
  log_file="$STATE_DIR/logs/egress-probe.log"
  "$EGRESS_PROBE_BINARY" \
    --origin "127.0.0.1:$EGRESS_ORIGIN_PORT" \
    --proxy "127.0.0.1:$EGRESS_PROXY_PORT" \
    --upstream-interface "$(upstream_interface)" \
    --upstream-resolver "1.1.1.1:53" \
    --mapped-target "$CONNECTION_REFRESH_TEST_HOST:443" \
    --mapped-upstream "127.0.0.1:$EGRESS_ORIGIN_PORT" \
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

start_control_api() {
  local api log_file store_dir token_file i
  api="http://127.0.0.1:$CONTROL_API_PORT"
  store_dir="$STATE_DIR/control-api"
  token_file="$store_dir/control-token"
  log_file="$STATE_DIR/logs/control-api.log"
  rm -rf "$store_dir"
  rm -f "$log_file"
  "$CONTROL_API_BINARY" --config "$CONFIG" --addr "127.0.0.1:$CONTROL_API_PORT" --store "$store_dir" >"$log_file" 2>&1 &
  CONTROL_API_PID=$!
  for i in {1..50}; do
    if [[ -s "$token_file" ]]; then
      CONTROL_API_TOKEN="$(cat "$token_file")"
      if /usr/bin/curl --fail --silent --show-error \
        --header "Authorization: Bearer $CONTROL_API_TOKEN" \
        "$api/api/v1/overview" >/dev/null 2>&1; then
        echo "Control API ready for connection refresh Lab: $api"
        return 0
      fi
    fi
    if ! kill -0 "$CONTROL_API_PID" 2>/dev/null; then
      echo "Control API exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
  echo "Control API did not become ready" >&2
  cat "$log_file" >&2 || true
  exit 1
}

stop_control_api() {
  if [[ -n "$CONTROL_API_PID" ]] && kill -0 "$CONTROL_API_PID" 2>/dev/null; then
    kill "$CONTROL_API_PID" 2>/dev/null || true
    wait "$CONTROL_API_PID" 2>/dev/null || true
  fi
  CONTROL_API_PID=""
  CONTROL_API_TOKEN=""
}

stop_egress_probe() {
  if [[ -n "$EGRESS_PROBE_PID" ]] && kill -0 "$EGRESS_PROBE_PID" 2>/dev/null; then
    kill "$EGRESS_PROBE_PID" 2>/dev/null || true
    wait "$EGRESS_PROBE_PID" 2>/dev/null || true
  fi
  EGRESS_PROBE_PID=""
}

build_http3_lab_binaries() {
  local go_cache go_path go_no_proxy
  go_cache="${GOCACHE:-/private/tmp/opensurge-http3-gocache}"
  go_path="${OMG_LAB_HTTP3_GOPATH:-/private/tmp/opensurge-http3-gopath}"
  go_no_proxy="${NO_PROXY:+$NO_PROXY,}goproxy.cn,proxy.golang.org,github.com,codeload.github.com,objects.githubusercontent.com"
  GOPATH="$go_path" GOMODCACHE="$go_path/pkg/mod" GOCACHE="$go_cache" \
    GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
    NO_PROXY="$go_no_proxy" no_proxy="$go_no_proxy" \
    go build -o "$HTTP3_PROBE_BINARY" ./tests/integration/http3probe
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    GOPATH="$go_path" GOMODCACHE="$go_path/pkg/mod" GOCACHE="$go_cache" \
    GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
    NO_PROXY="$go_no_proxy" no_proxy="$go_no_proxy" \
    go build -o "$HTTP3_CLIENT_BINARY" ./tests/integration/http3probe
}

install_http3_lab_client() {
  local client guest_tmp
  guest_tmp=/tmp/opensurge-http3-probe
  for client in $CLIENTS; do
    limactl copy --backend=scp "$HTTP3_CLIENT_BINARY" "$client:$guest_tmp"
    limactl shell "$client" -- sudo install -m 0755 "$guest_tmp" "$HTTP3_CLIENT_GUEST"
    limactl shell "$client" -- rm -f "$guest_tmp"
  done
}

start_http3_probe() {
  local log_file request_log i
  log_file="$STATE_DIR/logs/http3-probe.log"
  request_log="$STATE_DIR/egress/http3-origin.log"
  rm -f "$log_file" "$request_log"
  "$HTTP3_PROBE_BINARY" server \
    --listen "127.0.0.1:$IPV6_HTTP3_FIXTURE_PORT" \
    --log "$request_log" >"$log_file" 2>&1 &
  HTTP3_PROBE_PID=$!
  for i in {1..50}; do
    if grep -Fq 'READY protocol=h3' "$log_file" 2>/dev/null; then
      echo "HTTP/3-only Lab origin ready: udp://127.0.0.1:$IPV6_HTTP3_FIXTURE_PORT"
      return 0
    fi
    if ! kill -0 "$HTTP3_PROBE_PID" 2>/dev/null; then
      echo "HTTP/3-only Lab origin exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
  echo "HTTP/3-only Lab origin did not become ready" >&2
  cat "$log_file" >&2 || true
  exit 1
}

stop_http3_probe() {
  if [[ -n "$HTTP3_PROBE_PID" ]] && kill -0 "$HTTP3_PROBE_PID" 2>/dev/null; then
    kill "$HTTP3_PROBE_PID" 2>/dev/null || true
    wait "$HTTP3_PROBE_PID" 2>/dev/null || true
  fi
  HTTP3_PROBE_PID=""
}

write_ipv6_udp_proxy_config() {
  local config_dir
  config_dir="$STATE_DIR/ipv6-udp-proxy"
  mkdir -p "$config_dir"
  cat >"$config_dir/config.yaml" <<EOF
mixed-port: $IPV6_UDP_PROXY_PORT
allow-lan: false
bind-address: 127.0.0.1
mode: rule
log-level: info
ipv6: false
external-controller: ""
dns:
  enable: true
  listen: 127.0.0.1:$IPV6_UDP_PROXY_DNS_PORT
  nameserver:
    - 127.0.0.1:$IPV6_DNS_FIXTURE_PORT
rules:
  - MATCH,DIRECT
EOF
}

start_ipv6_udp_proxy() {
  local config_dir log_file i
  config_dir="$STATE_DIR/ipv6-udp-proxy"
  log_file="$STATE_DIR/logs/ipv6-udp-proxy.log"
  write_ipv6_udp_proxy_config
  rm -f "$log_file"
  mihomo -d "$config_dir" -f "$config_dir/config.yaml" >"$log_file" 2>&1 &
  IPV6_UDP_PROXY_PID=$!
  for i in {1..80}; do
    if /usr/bin/nc -z 127.0.0.1 "$IPV6_UDP_PROXY_PORT" >/dev/null 2>&1; then
      echo "controlled SOCKS5 UDP proxy ready: 127.0.0.1:$IPV6_UDP_PROXY_PORT"
      return 0
    fi
    if ! kill -0 "$IPV6_UDP_PROXY_PID" 2>/dev/null; then
      echo "controlled SOCKS5 UDP proxy exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
  echo "controlled SOCKS5 UDP proxy did not become ready" >&2
  cat "$log_file" >&2 || true
  exit 1
}

stop_ipv6_udp_proxy() {
  if [[ -n "$IPV6_UDP_PROXY_PID" ]] && kill -0 "$IPV6_UDP_PROXY_PID" 2>/dev/null; then
    kill "$IPV6_UDP_PROXY_PID" 2>/dev/null || true
    wait "$IPV6_UDP_PROXY_PID" 2>/dev/null || true
  fi
  IPV6_UDP_PROXY_PID=""
}

start_ipv6_dns_fixture() {
  local log_file upstream upstream_ipv4 i
  log_file="$STATE_DIR/logs/ipv6-dns-fixture.log"
  upstream="$(upstream_interface)"
  upstream_ipv4="$(/sbin/ifconfig "$upstream" | awk '$1 == "inet" { print $2; exit }')"
  [[ -n "$upstream_ipv4" ]] || {
    echo "upstream interface $upstream has no IPv4 source address for the IPv6 DNS fixture" >&2
    return 1
  }
  dnsmasq \
    --no-daemon \
    --conf-file=/dev/null \
    --no-resolv \
    --server="1.1.1.1@$upstream_ipv4" \
    --port="$IPV6_DNS_FIXTURE_PORT" \
    --listen-address=127.0.0.1 \
    --bind-interfaces \
    --address="/$IPV6_TCP_TEST_HOST/127.0.0.1" \
    --address="/$IPV6_UDP_TEST_HOST/127.0.0.1" \
    --address="/$IPV6_QUIC_TEST_HOST/127.0.0.1" \
    --address="/$IPV6_HTTP3_DIRECT_HOST/127.0.0.1" \
    --address="/$IPV6_HTTP3_PROXY_HOST/127.0.0.1" \
    --address="/$IPV6_HTTP3_BLOCKED_HOST/127.0.0.1" \
    --address="/$LOCAL_ROUTING_IPV6_HTTP3_HOST/192.0.2.123" \
    --address="/$IPV6_UDP_ANSWER_HOST/192.0.2.123" \
    --log-queries \
    --log-facility=- >"$log_file" 2>&1 &
  IPV6_DNS_FIXTURE_PID=$!
  for i in {1..50}; do
    if dig +time=1 +tries=1 +short @127.0.0.1 -p "$IPV6_DNS_FIXTURE_PORT" "$IPV6_TCP_TEST_HOST" A | grep -Fxq 127.0.0.1; then
      echo "IPv6 Lab DNS fixture ready: $IPV6_TCP_TEST_HOST=127.0.0.1"
      return 0
    fi
    if ! kill -0 "$IPV6_DNS_FIXTURE_PID" 2>/dev/null; then
      echo "IPv6 Lab DNS fixture exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
  echo "IPv6 Lab DNS fixture did not become ready" >&2
  cat "$log_file" >&2 || true
  exit 1
}

stop_ipv6_dns_fixture() {
  if [[ -n "$IPV6_DNS_FIXTURE_PID" ]] && kill -0 "$IPV6_DNS_FIXTURE_PID" 2>/dev/null; then
    kill "$IPV6_DNS_FIXTURE_PID" 2>/dev/null || true
    wait "$IPV6_DNS_FIXTURE_PID" 2>/dev/null || true
  fi
  IPV6_DNS_FIXTURE_PID=""
}

assert_tun_egress_proxy_unused() {
  local host
  host="$(url_host "$TEST_URL")"
  if grep -Fq -- "CONNECT $host:443" "$STATE_DIR/egress/proxy.log" 2>/dev/null; then
    echo "TunEgress DIRECT unexpectedly proxied CONNECT $host:443" >&2
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

client_ipv4() {
  local client=$1
  limactl shell "$client" -- bash -lc "ip -4 -o addr show dev omg0 scope global | awk 'NR == 1 { split(\$4, value, \"/\"); print value[1] }'" | tr -d '\r\n'
}

client_ipv6() {
  local client=$1
  limactl shell "$client" -- bash -lc "ip -6 -o addr show dev omg0 scope global | awk '/fdfe:dcba:9878:/ { split(\$4, value, \"/\"); print value[1]; exit }'" | tr -d '\r\n'
}

start_client_hold_connection() {
  local client=$1 log_file=$2
  rm -f "$log_file"
  limactl shell "$client" -- python3 -c \
    'import socket,sys,time; connection=socket.create_connection((sys.argv[1],443),10); print("READY",flush=True); time.sleep(180)' \
    "$CONNECTION_REFRESH_TEST_HOST" >"$log_file" 2>&1 &
  LAST_CLIENT_HOLD_PID=$!
}

stop_client_hold_connection() {
  local pid=$1
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

wait_for_client_hold_connection() {
  local client=$1 log_file=$2 pid=$3 i output
  for i in {1..40}; do
    output="$(cat "$log_file" 2>/dev/null || true)"
    if grep -Fq READY <<<"$output"; then
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 0.25
  done
  echo "client $client did not establish the held connection" >&2
  cat "$log_file" >&2 || true
  exit 1
}

fetch_mihomo_connections() {
  local output=$1
  /usr/bin/curl --fail --silent --show-error "http://127.0.0.1:19090/connections" >"$output"
}

connection_ids_for_source_host() {
  local snapshot=$1 source_ip=$2 host=$3 output=$4
  /usr/bin/ruby -rjson -e '
    body = JSON.parse(File.read(ARGV.fetch(0)))
    source, host = ARGV.fetch(1), ARGV.fetch(2)
    body.fetch("connections", []).each do |connection|
      metadata = connection.fetch("metadata", {})
      puts connection["id"] if metadata["sourceIP"].to_s == source && metadata["host"].to_s == host
    end
  ' "$snapshot" "$source_ip" "$host" >"$output"
}

wait_for_connection_ids() {
  local source_ip=$1 host=$2 snapshot=$3 output=$4 i
  for i in {1..40}; do
    fetch_mihomo_connections "$snapshot"
    connection_ids_for_source_host "$snapshot" "$source_ip" "$host" "$output"
    if [[ -s "$output" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "mihomo did not expose the held connection for $source_ip -> $host" >&2
  cat "$snapshot" >&2 || true
  exit 1
}

snapshot_contains_any_ids() {
  local snapshot=$1 ids=$2
  /usr/bin/ruby -rjson -rset -e '
    current = JSON.parse(File.read(ARGV.fetch(0))).fetch("connections", []).map { |connection| connection["id"].to_s }.to_set
    expected = File.readlines(ARGV.fetch(1), chomp: true).reject(&:empty?).to_set
    exit((current & expected).empty? ? 1 : 0)
  ' "$snapshot" "$ids"
}

wait_for_scoped_connection_refresh() {
  local target_ids=$1 other_ids=$2 snapshot=$3 i
  for i in {1..40}; do
    fetch_mihomo_connections "$snapshot"
    if ! snapshot_contains_any_ids "$snapshot" "$target_ids" && snapshot_contains_any_ids "$snapshot" "$other_ids"; then
      return 0
    fi
    sleep 0.25
  done
  echo "connection refresh did not remove only the target device connections" >&2
  cat "$snapshot" >&2 || true
  exit 1
}

assert_connection_refresh_response() {
  local response=$1 device_id=$2
  /usr/bin/ruby -rjson -e '
    payload = JSON.parse(File.read(ARGV.fetch(0)))
    device = ARGV.fetch(1)
    abort "unexpected refresh scope" unless payload["scope"] == "device" && payload["device_id"] == device
    matched = Integer(payload.fetch("matched_connections"))
    closed = Integer(payload.fetch("closed_connections"))
    abort "refresh did not close every matched connection" unless matched.positive? && closed == matched
  ' "$response" "$device_id"
}

wait_for_ipv6_policy_log() {
  local network=$1 source_ip=$2 target=$3 action=$4 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..30}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "[$network]" "$log_file" | grep -F -- "$source_ip" | grep -F -- "--> $target" | grep -Fq -- "using $action"; then
      echo "IPv6 $network policy log observed for $source_ip -> $target using $action"
      return 0
    fi
    sleep 0.2
  done
  echo "mihomo did not log IPv6 $network traffic for $source_ip -> $target using $action" >&2
  tail -180 "$log_file" >&2 || true
  exit 1
}

run_ipv6_http3_client() {
  local client source host request_path evidence_file
  client="$1"
  source="$2"
  host="$3"
  request_path="$4"
  evidence_file="$5"
  limactl shell "$client" -- bash -c '
    set -euo pipefail
    source_ip="$1"
    host="$2"
    port="$3"
    request_path="$4"
    probe="$5"
    iface=omg0
    dns="$(awk '\''$1 == "nameserver" && $2 ~ /:/ { print $2; exit }'\'' /etc/resolv.conf 2>/dev/null || true)"
    if [[ -z "$dns" ]]; then
      dns="$(ip -6 route show default dev "$iface" | awk '\''$1 == "default" && $2 == "via" { print $3; exit }'\'')"
    fi
    [[ -n "$dns" ]] || { echo "no IPv6 DNS endpoint on $iface" >&2; exit 1; }
    if [[ "$dns" == fe80:* ]]; then
      dns="$dns%$iface"
    fi
    fake="$(dig +time=5 +tries=1 +short "@$dns" "$host" AAAA | awk '\''/^fdfe:dcba:9876:/ { print; exit }'\'')"
    [[ -n "$fake" ]]
    ip -6 route get "$fake" | grep -q "dev $iface"
    "$probe" client \
      --url "https://$host:$port$request_path" \
      --address "$fake" \
      --source "$source_ip"
  ' _ "$source" "$host" "$IPV6_HTTP3_FIXTURE_PORT" "$request_path" "$HTTP3_CLIENT_GUEST" \
    >"$STATE_DIR/egress/$evidence_file" 2>&1
}

wait_for_http3_origin() {
  local host request_path i log_file
  host="$1"
  request_path="$2"
  log_file="$STATE_DIR/egress/http3-origin.log"
  for i in {1..30}; do
    if [[ -f "$log_file" ]] &&
      grep -Fq "HTTP3 GET $request_path proto=HTTP/3.0 host=$host:$IPV6_HTTP3_FIXTURE_PORT" "$log_file"; then
      echo "HTTP/3 origin evidence observed for $host$request_path"
      return 0
    fi
    sleep 0.2
  done
  echo "HTTP/3 origin did not record $host$request_path" >&2
  cat "$log_file" >&2 || true
  exit 1
}

wait_for_controlled_udp_proxy() {
  local i log_file
  log_file="$STATE_DIR/logs/ipv6-udp-proxy.log"
  for i in {1..30}; do
    if [[ -f "$log_file" ]] && grep -Fq '[UDP]' "$log_file"; then
      echo "controlled SOCKS5 UDP relay evidence observed"
      return 0
    fi
    sleep 0.2
  done
  echo "controlled SOCKS5 proxy did not record UDP relay traffic" >&2
  tail -160 "$log_file" >&2 || true
  exit 1
}

assert_ipv6_bpf_bidirectional() {
  local log_file
  log_file="$STATE_DIR/logs/ipv6-packet.log"
  grep -Fq 'OpenSurge IPv6 packet ingress accepted=' "$log_file" || {
    echo "IPv6 BPF broker did not record accepted ingress" >&2
    tail -160 "$log_file" >&2 || true
    exit 1
  }
  grep -Fq 'OpenSurge IPv6 packet egress written=' "$log_file" || {
    echo "IPv6 BPF broker did not record written egress" >&2
    tail -160 "$log_file" >&2 || true
    exit 1
  }
  echo "IPv6 BPF broker ingress and egress evidence observed"
}

build_ipv6_lab_binaries() {
  local build_cache build_log go_no_proxy mod_cache
  require_command go
  # The optional Lab proxy file may outlive the LAN proxy that created it.
  # The pinned Go module mirror is directly reachable in the supported setup,
  # so do not let a stale general-purpose proxy turn a build prerequisite into
  # a misleading IPv6 data-plane failure.
  go_no_proxy="${NO_PROXY:+$NO_PROXY,}goproxy.cn,proxy.golang.org,github.com,codeload.github.com,objects.githubusercontent.com"
  build_cache="${GOCACHE:-/private/tmp/opensurge-v020-mihomo-cache}"
  mod_cache="${GOMODCACHE:-/private/tmp/opensurge-ipv6-modcache}"
  build_log="$STATE_DIR/logs/mihomo-build.log"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
  NO_PROXY="$go_no_proxy" \
  no_proxy="$go_no_proxy" \
    go build -o "$IPV6_PACKET_BINARY" ./cmd/opensurge-network
  if ! GOCACHE="$build_cache" \
    GOMODCACHE="$mod_cache" \
    GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
    NO_PROXY="$go_no_proxy" \
    no_proxy="$go_no_proxy" \
    OPENSURGE_MIHOMO_OUTPUT="$PATCHED_MIHOMO_BINARY" \
      "$ROOT/scripts/build-opensurge-mihomo.sh" 2>&1 | tee "$build_log"; then
    if grep -F "$mod_cache" "$build_log" | grep -Eq 'no such file or directory|no matching files found' ||
      grep -Fq 'no required module provides package' "$build_log"; then
      echo "OpenSurge Mihomo build caches are inconsistent; rebuilding the disposable Lab caches"
      GOMODCACHE="$mod_cache" go clean -modcache
      GOCACHE="$build_cache" go clean -cache
    else
      return 1
    fi
    GOCACHE="$build_cache" \
    GOMODCACHE="$mod_cache" \
    GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
    NO_PROXY="$go_no_proxy" \
    no_proxy="$go_no_proxy" \
    OPENSURGE_MIHOMO_OUTPUT="$PATCHED_MIHOMO_BINARY" \
      "$ROOT/scripts/build-opensurge-mihomo.sh" 2>&1 | tee "$build_log"
  fi
}

write_ipv6_device_policy_fixture() {
  local topology client_one client_two mac_one mac_two gateway_target
  topology="${1:-isolated_lan}"
  set -- $CLIENTS
  [[ "$#" -eq 2 ]] || { echo "IPv6 lab requires exactly two clients" >&2; exit 1; }
  client_one="$1"
  client_two="$2"
  mac_one="$(client_mac "$client_one")"
  mac_two="$(client_mac "$client_two")"
  gateway_target=""
  if [[ "$topology" == "same_wifi_dhcp" ]]; then
    gateway_target=',"gateway_target":"upstream_router"'
  fi
  LAB_DEVICE_POLICY_FILE="$STATE_DIR/device-policy.ipv6.json"
  cat >"$LAB_DEVICE_POLICY_FILE" <<EOF
{
  "profiles": [
    {"id":"blocked","default_policies":["DIRECT"],"rules":[{"id":"block-ipv6-lab","match":{"domains":["$IPV6_TCP_TEST_HOST"]},"action":"REJECT"}]},
    {"id":"direct","default_policies":["DIRECT","lab-udp-proxy","lab-http-only"]}
  ],
  "devices": [
    {"id":"$client_one","mac":"$mac_one","ipv4":"192.168.50.101","profile":"blocked","egress_mode":"dedicated"$gateway_target},
    {"id":"$client_two","mac":"$mac_two","ipv4":"192.168.50.102","profile":"direct","egress_mode":"dedicated"},
    {"id":"dormant-old-lan","mac":"02:00:00:60:01:50","ipv4":"192.168.60.150","profile":"blocked","egress_mode":"dedicated"}
  ]
}
EOF
  LAB_MIHOMO_PROFILE="$STATE_DIR/mihomo-profile.ipv6.yaml"
  cat >"$LAB_MIHOMO_PROFILE" <<EOF
proxies:
  - name: lab-udp-proxy
    type: socks5
    server: 127.0.0.1
    port: $IPV6_UDP_PROXY_PORT
    udp: true
  - name: lab-http-only
    type: http
    server: 127.0.0.1
    port: $EGRESS_PROXY_PORT
dns:
  nameserver:
    - 127.0.0.1:$IPV6_DNS_FIXTURE_PORT
rules:
  - MATCH,DIRECT
EOF
}

write_ipv6_real_profile_and_policy() {
  local client_one client_two mac_one mac_two profile_source
  [[ -n "$IPV6_REAL_PROFILE_SOURCE" ]] || {
    echo "OMG_LAB_IPV6_REAL_PROFILE is required for the real-subscription IPv6 gate" >&2
    exit 1
  }
  profile_source="$(resolve_lab_profile "$IPV6_REAL_PROFILE_SOURCE")"
  [[ -f "$profile_source" ]] || { echo "real IPv6 mihomo profile not found" >&2; exit 1; }
  [[ "$IPV6_REAL_TCP_HOST" =~ ^[A-Za-z0-9.-]+$ ]] || { echo "invalid real IPv6 TCP host" >&2; exit 1; }
  [[ "$IPV6_NATIVE_DIRECT_HOST" =~ ^[A-Za-z0-9.-]+$ ]] || { echo "invalid native IPv6 DIRECT host" >&2; exit 1; }
  /usr/bin/ruby -ripaddr -e 'IPAddr.new(ARGV.fetch(0)).ipv6? or exit 1' "$IPV6_REAL_TARGET" || {
    echo "invalid real IPv6 target" >&2
    exit 1
  }
  /usr/bin/ruby -ripaddr -e 'IPAddr.new(ARGV.fetch(0)).ipv6? or exit 1' "$IPV6_REAL_UDP_TARGET" || {
    echo "invalid real IPv6 UDP target" >&2
    exit 1
  }
  /usr/bin/ruby -ripaddr -e 'IPAddr.new(ARGV.fetch(0)).ipv6? or exit 1' "$IPV6_NATIVE_DIRECT_TARGET" || {
    echo "invalid native IPv6 DIRECT target" >&2
    exit 1
  }

  set -- $CLIENTS
  [[ "$#" -eq 2 ]] || { echo "IPv6 lab requires exactly two clients" >&2; exit 1; }
  client_one="$1"
  client_two="$2"
  mac_one="$(client_mac "$client_one")"
  mac_two="$(client_mac "$client_two")"

  install -m 0600 "$profile_source" "$IPV6_REAL_PROFILE"
  LAB_MIHOMO_PROFILE="$IPV6_REAL_PROFILE"
  LAB_DEVICE_POLICY_FILE="$STATE_DIR/device-policy.ipv6-real.json"
  cat >"$LAB_DEVICE_POLICY_FILE" <<EOF
{
  "profiles": [
    {
      "id":"blocked",
      "default_policies":["DIRECT"],
      "rules":[
        {"id":"block-real-ipv6","match":{"domains":["$IPV6_REAL_TCP_HOST"]},"action":"REJECT"},
        {"id":"native-direct-https","match":{"domains":["$IPV6_NATIVE_DIRECT_HOST"]},"action":"DIRECT"},
        {"id":"native-direct-udp","match":{"protocols":["udp"],"ports":["53","443"]},"action":"DIRECT"}
      ]
    },
    {"id":"inherit","default_policies":["DIRECT"]}
  ],
  "devices": [
    {"id":"$client_one","mac":"$mac_one","ipv4":"192.168.50.101","profile":"blocked","egress_mode":"inherit_global"},
    {"id":"$client_two","mac":"$mac_two","ipv4":"192.168.50.102","profile":"inherit","egress_mode":"inherit_global"}
  ]
}
EOF
}

native_ipv6_exit_address() {
  local iface
  iface="$(upstream_interface)"
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
    -u http_proxy -u https_proxy -u all_proxy \
    /usr/bin/curl -6 --noproxy '*' --interface "$iface" \
      --connect-timeout 8 --max-time 20 --fail --silent --show-error \
      "$IPV6_NATIVE_DIRECT_URL"
}

assert_native_ipv6_exit_on_upstream() {
  local exit_address=$1 iface
  /usr/bin/ruby -ripaddr -e 'IPAddr.new(ARGV.fetch(0)).ipv6? or exit 1' "$exit_address" || {
    echo "native IPv6 DIRECT probe did not return an IPv6 address" >&2
    return 1
  }
  iface="$(upstream_interface)"
  /sbin/ifconfig "$iface" | awk -v expected="$exit_address" '
    $1 == "inet6" {
      address = $2
      sub(/%.*/, "", address)
      if (address == expected) found = 1
    }
    END { exit(found ? 0 : 1) }
  ' || {
    echo "native IPv6 DIRECT exit was not assigned to the selected Mac upstream interface" >&2
    return 1
  }
}

profile_proxy_records() {
  /usr/bin/ruby -ryaml -rdate -rbase64 -e '
    data = YAML.safe_load(File.read(ARGV.fetch(0)), permitted_classes: [Date, Time], aliases: true)
    Array(data["proxies"]).each do |proxy|
      next unless proxy.is_a?(Hash) && proxy["name"] && proxy["type"]
      name = proxy["name"].to_s.encode("UTF-8")
      type = proxy["type"].to_s.encode("UTF-8")
      STDOUT.write(type, "\t", Base64.strict_encode64(name), "\n")
    end
  ' "$IPV6_REAL_PROFILE"
}

socks5_ipv6_udp_dns_probe() {
  /usr/bin/python3 -c '
import ipaddress
import os
import socket
import struct
import sys

proxy_host, proxy_port, target, query_name = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]

def recv_exact(sock, size):
    body = b""
    while len(body) < size:
        chunk = sock.recv(size - len(body))
        if not chunk:
            raise RuntimeError("SOCKS5 control connection closed")
        body += chunk
    return body

control = socket.create_connection((proxy_host, proxy_port), timeout=8)
control.settimeout(8)
control.sendall(b"\x05\x01\x00")
if recv_exact(control, 2) != b"\x05\x00":
    raise RuntimeError("SOCKS5 proxy rejected no-auth negotiation")
control.sendall(b"\x05\x03\x00\x01\x00\x00\x00\x00\x00\x00")
version, reply, _, atyp = recv_exact(control, 4)
if version != 5 or reply != 0:
    raise RuntimeError("SOCKS5 UDP ASSOCIATE failed")
if atyp == 1:
    relay_host = socket.inet_ntop(socket.AF_INET, recv_exact(control, 4))
elif atyp == 4:
    relay_host = socket.inet_ntop(socket.AF_INET6, recv_exact(control, 16))
elif atyp == 3:
    relay_host = recv_exact(control, recv_exact(control, 1)[0]).decode("ascii")
else:
    raise RuntimeError("SOCKS5 UDP relay returned an unknown address type")
relay_port = struct.unpack("!H", recv_exact(control, 2))[0]
try:
    if ipaddress.ip_address(relay_host).is_unspecified:
        relay_host = proxy_host
except ValueError:
    pass

transaction = os.urandom(2)
qname = b"".join(bytes([len(label)]) + label.encode("ascii") for label in query_name.rstrip(".").split(".")) + b"\x00"
dns_query = transaction + b"\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00" + qname + b"\x00\x1c\x00\x01"
udp_family = socket.AF_INET6 if ":" in relay_host else socket.AF_INET
udp = socket.socket(udp_family, socket.SOCK_DGRAM)
udp.settimeout(12)
request = b"\x00\x00\x00\x04" + socket.inet_pton(socket.AF_INET6, target) + struct.pack("!H", 53) + dns_query
udp.sendto(request, (relay_host, relay_port))
response, _ = udp.recvfrom(65535)
if response[:3] != b"\x00\x00\x00":
    raise RuntimeError("invalid SOCKS5 UDP response header")
response_atyp = response[3]
if response_atyp == 1:
    offset = 10
elif response_atyp == 4:
    offset = 22
elif response_atyp == 3:
    offset = 7 + response[4]
else:
    raise RuntimeError("unknown SOCKS5 UDP response address type")
dns_response = response[offset:]
if len(dns_response) < 12 or dns_response[:2] != transaction or not (dns_response[2] & 0x80):
    raise RuntimeError("invalid DNS response through SOCKS5 UDP")
if struct.unpack("!H", dns_response[6:8])[0] < 1:
    raise RuntimeError("SOCKS5 UDP DNS response had no answers")
udp.close()
control.close()
' 127.0.0.1 17890 "$IPV6_REAL_UDP_TARGET" "$IPV6_REAL_UDP_QUERY"
}

select_working_real_proxy() {
  local direct_exit=$1 api proxy_url attempted proxy_type encoded_name proxy_name payload proxy_exit exit_family literal_code
  api="http://127.0.0.1:19090"
  proxy_url="http://127.0.0.1:17890"
  /usr/bin/curl --fail --silent --show-error --request PATCH \
    --header 'Content-Type: application/json' \
    --data '{"mode":"global","log-level":"info"}' "$api/configs" >/dev/null

  attempted=0
  while IFS=$'\t' read -r proxy_type encoded_name; do
    [[ "$attempted" -lt 8 ]] || break
    attempted=$((attempted + 1))
    proxy_name="$(printf '%s' "$encoded_name" | /usr/bin/base64 -D)"
    payload="$(/usr/bin/ruby -rjson -e 'name=ARGV.fetch(0).dup.force_encoding("UTF-8"); print JSON.generate({name: name})' "$proxy_name")"
    if ! /usr/bin/curl --fail --silent --request PUT \
      --header 'Content-Type: application/json' --data "$payload" \
      "$api/proxies/GLOBAL" >/dev/null 2>&1; then
      echo "real subscription candidate $attempted ($proxy_type) could not be selected"
      continue
    fi
    proxy_exit="$(env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
      -u http_proxy -u https_proxy -u all_proxy \
      /usr/bin/curl --proxy "$proxy_url" --connect-timeout 8 --max-time 20 \
        --fail --silent "$IPV6_REAL_TCP_URL" 2>/dev/null || true)"
    if [[ "$proxy_exit" == *:* ]]; then
      exit_family=ipv6
    elif [[ "$proxy_exit" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      exit_family=ipv4
    else
      echo "real subscription candidate $attempted ($proxy_type) failed HTTPS"
      continue
    fi
    literal_code="$(env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
      -u http_proxy -u https_proxy -u all_proxy \
      /usr/bin/curl --proxy "$proxy_url" --connect-timeout 8 --max-time 20 \
        --silent --insecure --output /dev/null --write-out '%{http_code}' \
        "$IPV6_REAL_TARGET_URL" 2>/dev/null || true)"
    if [[ "$proxy_exit" == "$direct_exit" ]] || [[ ! "$literal_code" =~ ^[1-5][0-9][0-9]$ ]]; then
      echo "real subscription candidate $attempted ($proxy_type) did not prove a distinct IPv6-capable egress"
      continue
    fi
    if ! socks5_ipv6_udp_dns_probe >/dev/null 2>&1; then
      echo "real subscription candidate $attempted ($proxy_type) did not provide IPv6 UDP DNS egress"
      continue
    fi
    REAL_PROXY_TYPE="$proxy_type"
    REAL_PROXY_INDEX="$attempted"
    REAL_PROXY_EXIT_FAMILY="$exit_family"
    echo "real subscription candidate $attempted ($proxy_type) passed HTTPS, IPv6-literal, and UDP egress"
    return 0
  done < <(profile_proxy_records)
  echo "no tested real subscription candidate provided distinct HTTPS, IPv6-literal, and UDP egress" >&2
  return 1
}

wait_for_ipv6_global_log() {
  local network=$1 source_ip=$2 target=$3 port=$4 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  [[ -n "$REAL_PROXY_INDEX" ]] || {
    echo "real subscription proxy selection is unavailable for log verification" >&2
    return 1
  }
  for i in {1..40}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "[$network]" "$log_file" | grep -F -- "$source_ip" |
        grep -F -- "$target" | grep -F -- ":$port" | grep -Fq -- 'using GLOBAL'; then
      echo "IPv6 $network real subscription egress observed for device source on port $port"
      return 0
    fi
    sleep 0.25
  done
  echo "mihomo did not record the selected real subscription egress for IPv6 $network on port $port" >&2
  print_sanitized_ipv6_log_context "$source_ip"
  return 1
}

wait_for_ipv6_direct_log() {
  local network=$1 source_ip=$2 target=$3 port=$4 i log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  for i in {1..40}; do
    if [[ -f "$log_file" ]] &&
      grep -F -- "[$network]" "$log_file" | grep -F -- "$source_ip" |
        grep -F -- "$target" | grep -F -- ":$port" | grep -Fq -- 'using DIRECT'; then
      echo "IPv6 $network native DIRECT egress observed for device source on port $port"
      return 0
    fi
    sleep 0.25
  done
  echo "mihomo did not record native IPv6 $network DIRECT egress on port $port" >&2
  print_sanitized_ipv6_log_context "$source_ip"
  return 1
}

print_sanitized_ipv6_log_context() {
  local source_ip=$1 log_file
  log_file="$STATE_DIR/logs/mihomo.log"
  [[ -f "$log_file" && -f "$IPV6_REAL_PROFILE" ]] || return 0
  /usr/bin/ruby -ryaml -rdate -e '
    profile_path, log_path, source = ARGV
    profile = YAML.safe_load(File.read(profile_path), permitted_classes: [Date, Time], aliases: true)
    markers = []
    Array(profile["proxies"]).each do |proxy|
      next unless proxy.is_a?(Hash)
      proxy.each_value { |value| markers << value.to_s if value.is_a?(String) && value.bytesize >= 4 }
    end
    Array(profile["proxy-groups"]).each do |group|
      next unless group.is_a?(Hash)
      name = group["name"].to_s
      markers << name if name.bytesize >= 4
    end
    lines = File.binread(log_path).lines.select { |line| line.include?(source.b) }.last(12)
    exit 0 if lines.empty?
    warn "sanitized Mihomo context for the device source follows:"
    markers.uniq.sort_by { |marker| -marker.bytesize }.each do |marker|
      lines.each { |line| line.gsub!(marker.b, "[subscription-redacted]") }
    end
    lines.each { |line| STDERR.write(line) }
  ' "$IPV6_REAL_PROFILE" "$log_file" "$source_ip"
}

assert_client_ipv6_withdrawn() {
  local client=$1 i
  for i in {1..30}; do
    if ! limactl shell "$client" -- bash -lc "ip -6 route show default dev omg0 | grep -q '^default '"; then
      echo "$client withdrew the OpenSurge IPv6 default route"
      return 0
    fi
    sleep 0.2
  done
  echo "$client retained the OpenSurge IPv6 default route after gateway stop" >&2
  limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status >&2 || true
  exit 1
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

assert_client_ipv4_prefix() {
  local client=$1 expected=$2 actual
  actual="$(limactl shell "$client" -- bash -lc "ip -4 -o addr show dev omg0 scope global | awk 'NR == 1 { print \$4 }'" | tr -d '\r\n')"
  if [[ "$actual" != "$expected" ]]; then
    echo "$client IPv4 prefix $actual, want $expected" >&2
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client status >&2 || true
    exit 1
  fi
}

assert_lab_ipv4_prefix() {
  local iface
  iface="$(lab_interface)"
  /sbin/ifconfig "$iface" | grep -F "inet $LAN_IP netmask 0xfffffc00" >/dev/null || {
    echo "lab interface $iface is not configured as $LAN_IP/22" >&2
    /sbin/ifconfig "$iface" >&2
    exit 1
  }
}

assert_device_policy_identity_ready() {
  local output=$1 digest
  [[ -f "$STATE_DIR/device-policy.applied.json" ]] || {
    echo "applied device-policy snapshot was not written" >&2
    exit 1
  }
  [[ -f "$STATE_DIR/state.json" ]] || {
    echo "runtime state was not written" >&2
    exit 1
  }
  grep -Fq '"policy_source": "applied"' "$output"
  [[ "$(grep -Fc '"policy_identity_ready": true' "$output")" -eq 2 ]] || {
    echo "both DHCP reservations were not policy identity ready" >&2
    cat "$output" >&2
    exit 1
  }
  [[ "$(grep -Fc '"lease_match": true' "$output")" -eq 2 ]] || {
    echo "both DHCP leases did not match policy identity" >&2
    cat "$output" >&2
    exit 1
  }
  digest="$(sed -n 's/  "digest": "\([^"]*\)",/\1/p' "$STATE_DIR/device-policy.applied.json" | head -1)"
  [[ -n "$digest" ]] || { echo "applied policy snapshot has no digest" >&2; exit 1; }
  grep -Fq "\"device_policy_digest\": \"$digest\"" "$STATE_DIR/state.json"
}

assert_ip_only_device_paused() {
  local output=$1
  grep -Fq 'paused-ip-only' "$STATE_DIR/device-policy.applied.json" || {
    echo "DHCP mode did not preserve the raw IP-only policy record" >&2
    exit 1
  }
  if grep -Fq 'paused-ip-only' "$output"; then
    echo "DHCP mode unexpectedly compiled the IP-only device" >&2
    exit 1
  fi
  if grep -Fq '192.168.50.150' "$STATE_DIR/dnsmasq.conf"; then
    echo "DHCP mode unexpectedly emitted a reservation for the IP-only device" >&2
    exit 1
  fi
  if grep -Fq '192.168.50.150/32' "$STATE_DIR/mihomo.yaml"; then
    echo "DHCP mode unexpectedly emitted a routing rule for the IP-only device" >&2
    exit 1
  fi
}

assert_out_of_lan_device_dormant() {
  local output=${1:-}
  /usr/bin/ruby -rjson -e '
    snapshot = JSON.parse(File.read(ARGV.fetch(0)))
    abort "applied snapshot has wrong active LAN" unless snapshot["active_lan"] == "192.168.48.0/22"
    desired = snapshot.fetch("policy").fetch("devices")
    abort "dormant desired record was not preserved" unless desired.any? { |item| item["id"] == "dormant-old-lan" && item["ipv4"] == "192.168.60.150" }
    compiled = JSON.generate(snapshot.fetch("compiled"))
    abort "dormant device leaked into compiled runtime" if compiled.include?("dormant-old-lan") || compiled.include?("192.168.60.150") || compiled.include?("02:00:00:60:01:50")
  ' "$STATE_DIR/device-policy.applied.json"
  for path in "$STATE_DIR/dnsmasq.conf" "$STATE_DIR/mihomo.yaml"; do
    if grep -Eq 'dormant-old-lan|192\.168\.60\.150|02:00:00:60:01:50' "$path"; then
      echo "out-of-LAN device leaked into generated runtime config $path" >&2
      exit 1
    fi
  done
  if [[ -n "$output" ]] && grep -Fq 'dormant-old-lan' "$output"; then
    echo "out-of-LAN device leaked into active runtime device list" >&2
    exit 1
  fi
}

assert_applied_policy_drift() {
  local output=$1
  grep -Fq '"policy_source": "applied"' "$output"
  grep -Fq '"drift": true' "$output"
  grep -Fq '"ipv4": "192.168.50.101"' "$output"
}

assert_applied_policy_synced() {
  local output=$1 desired applied
  grep -Fq '"policy_source": "applied"' "$output"
  grep -Fq '"drift": false' "$output"
  desired="$(sed -n 's/.*"desired_digest": "\([^"]*\)".*/\1/p' "$output" | head -1)"
  applied="$(sed -n 's/.*"applied_digest": "\([^"]*\)".*/\1/p' "$output" | head -1)"
  [[ -n "$desired" && "$desired" == "$applied" ]] || {
    echo "reload did not synchronize desired/applied device-policy digests" >&2
    cat "$output" >&2
    exit 1
  }
}

mutate_device_policy_desired() {
  /usr/bin/perl -0pi -e 's/"default_policies": \["lab-controlled", "DIRECT"\]/"default_policies": ["DIRECT", "lab-controlled"]/' "$LAB_DEVICE_POLICY_FILE"
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
rules: ['MATCH,DIRECT']
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
      "profile": "controlled",
      "egress_mode": "dedicated"
    },
    {
      "id": "$client_two",
      "mac": "$mac_two",
      "ipv4": "192.168.51.102",
      "profile": "direct-blocked",
      "egress_mode": "inherit_global"
    },
    {
      "id": "paused-ip-only",
      "ipv4": "192.168.50.150",
      "profile": "controlled",
      "egress_mode": "dedicated"
    },
    {
      "id": "dormant-old-lan",
      "mac": "02:00:00:60:01:50",
      "ipv4": "192.168.60.150",
      "profile": "controlled",
      "egress_mode": "dedicated"
    }
  ]
}
EOF
}

write_device_block_rule() {
  local client_one=$1 client_two=$2 mac_one mac_two
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
          "id": "block-test-ip",
          "match": {"ip_cidrs": ["1.1.1.1/32"]},
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
      "profile": "controlled",
      "egress_mode": "dedicated"
    },
    {
      "id": "$client_two",
      "mac": "$mac_two",
      "ipv4": "192.168.51.102",
      "profile": "direct-blocked",
      "egress_mode": "dedicated"
    },
    {
      "id": "paused-ip-only",
      "ipv4": "192.168.50.150",
      "profile": "controlled",
      "egress_mode": "dedicated"
    },
    {
      "id": "dormant-old-lan",
      "mac": "02:00:00:60:01:50",
      "ipv4": "192.168.60.150",
      "profile": "controlled",
      "egress_mode": "dedicated"
    }
  ]
}
EOF
}

run_device_policy_test() {
  local client_one client_two client_one_hold_pid client_two_hold_pid gateway_started egress_probe_started control_api_started hold_connections_started host
  require_installed_lab
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_cached_sudo
  start_sudo_keepalive
  trap stop_sudo_keepalive EXIT
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
  build_control_api

  gateway_started=0
  egress_probe_started=0
  control_api_started=0
  hold_connections_started=0
  client_one_hold_pid=""
  client_two_hold_pid=""
  cleanup_test() {
    status=$?
    collect_artifacts || true
    restore_client_control_dns
    if [[ "$hold_connections_started" == 1 ]]; then
      stop_client_hold_connection "$client_one_hold_pid"
      stop_client_hold_connection "$client_two_hold_pid"
    fi
    if [[ "$control_api_started" == 1 ]]; then
      stop_control_api
    fi
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    if [[ "$egress_probe_started" == 1 ]]; then
      stop_egress_probe || true
    fi
    stop_sudo_keepalive
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  start_egress_probe
  egress_probe_started=1
  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1
  start_control_api
  control_api_started=1
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  assert_lab_ipv4_prefix
  assert_client_ipv4 "$client_one" "192.168.50.101"
  assert_client_ipv4 "$client_two" "192.168.51.102"
  assert_client_ipv4_prefix "$client_one" "192.168.50.101/22"
  assert_client_ipv4_prefix "$client_two" "192.168.51.102/22"
  "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/device-policies.json"
  grep -Fq '"ipv4": "192.168.50.101"' "$STATE_DIR/device-policies.json"
  grep -Fq '"ipv4": "192.168.51.102"' "$STATE_DIR/device-policies.json"
  grep -Fq '"egress_mode": "dedicated"' "$STATE_DIR/device-policies.json"
  grep -Fq '"egress_mode": "inherit_global"' "$STATE_DIR/device-policies.json"
  assert_device_policy_identity_ready "$STATE_DIR/device-policies.json"
  assert_ip_only_device_paused "$STATE_DIR/device-policies.json"
  assert_out_of_lan_device_dormant "$STATE_DIR/device-policies.json"
  if "$BINARY" device-policy-select --config "$CONFIG" --device dormant-old-lan --slot default --policy DIRECT --format json >"$STATE_DIR/dormant-device-select.json" 2>&1; then
    echo "out-of-LAN device unexpectedly accepted a runtime selector change" >&2
    exit 1
  fi
  grep -Fq 'unknown device' "$STATE_DIR/dormant-device-select.json"

  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client udp "$LAN_IP" 1.1.1.1 443
  wait_for_tun_udp_reject "192.168.50.101" "1.1.1.1" 443

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "lab-controlled" "$host"
  assert_tun_egress_proxy_used

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_action_log "$host" "DIRECT" "192.168.51.102"
  assert_tun_egress_proxy_unused
  "$BINARY" policies --config "$CONFIG" --format json >"$STATE_DIR/device-policies-initial-live.json"
  if grep -Fq "\"name\": \"device/$client_two/default\"" "$STATE_DIR/device-policies-initial-live.json"; then
    echo "inherit_global device unexpectedly exposed a default selector" >&2
    exit 1
  fi
  if "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy lab-controlled --format json >"$STATE_DIR/device-two-inherit-select.json" 2>&1; then
    echo "inherit_global device unexpectedly accepted a default selector change" >&2
    exit 1
  fi
  grep -Fq 'has no selectable policy slot' "$STATE_DIR/device-two-inherit-select.json"

  mutate_device_policy_desired
  "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/device-policies-drift.json"
  assert_applied_policy_drift "$STATE_DIR/device-policies-drift.json"

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_one" --slot default --policy DIRECT --format json >"$STATE_DIR/device-one-direct.json"
  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "DIRECT" "$host"
  assert_tun_egress_proxy_unused

  "$BINARY" policies --config "$CONFIG" --format json >"$STATE_DIR/device-policies-live.json"
  grep -A 4 -F "\"name\": \"device/$client_one/default\"" "$STATE_DIR/device-policies-live.json" | grep -Fq '"selected": "DIRECT"'
  if grep -Fq "\"name\": \"device/$client_two/default\"" "$STATE_DIR/device-policies-live.json"; then
    echo "inherit_global device gained a default selector before reload" >&2
    exit 1
  fi

  write_device_block_rule "$client_one" "$client_two"
  "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/device-policies-reload-drift.json"
  assert_applied_policy_drift "$STATE_DIR/device-policies-reload-drift.json"
  sudo -n "$BINARY" reload --config "$CONFIG" --format json >"$STATE_DIR/device-policy-reload.json"
  grep -Fq '"command": "reload"' "$STATE_DIR/device-policy-reload.json"
  grep -Fq '"ok": true' "$STATE_DIR/device-policy-reload.json"
  "$BINARY" status --config "$CONFIG" --format json >"$STATE_DIR/device-policy-status-after-reload.json"
  grep -Fq '"gateway": "running"' "$STATE_DIR/device-policy-status-after-reload.json"
  "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/device-policies-after-reload.json"
  assert_applied_policy_synced "$STATE_DIR/device-policies-after-reload.json"
  assert_device_policy_identity_ready "$STATE_DIR/device-policies-after-reload.json"
  assert_ip_only_device_paused "$STATE_DIR/device-policies-after-reload.json"
  assert_out_of_lan_device_dormant "$STATE_DIR/device-policies-after-reload.json"

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_one" --slot default --policy DIRECT --format json >"$STATE_DIR/device-one-direct-after-reload.json"
  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy lab-controlled --format json >"$STATE_DIR/device-two-controlled-after-reload.json"
  "$BINARY" policies --config "$CONFIG" --format json >"$STATE_DIR/device-policies-live-after-reload.json"
  grep -A 4 -F "\"name\": \"device/$client_one/default\"" "$STATE_DIR/device-policies-live-after-reload.json" | grep -Fq '"selected": "DIRECT"'
  grep -A 4 -F "\"name\": \"device/$client_two/default\"" "$STATE_DIR/device-policies-live-after-reload.json" | grep -Fq '"selected": "lab-controlled"'

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "DIRECT" "$host"
  assert_tun_egress_proxy_unused

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_two/default" "lab-controlled" "$host"
  assert_tun_egress_proxy_used

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_one" --slot default --policy lab-controlled --format json >"$STATE_DIR/device-one-controlled-for-refresh.json"
  start_client_hold_connection "$client_one" "$STATE_DIR/logs/connection-refresh-$client_one.log"
  client_one_hold_pid="$LAST_CLIENT_HOLD_PID"
  start_client_hold_connection "$client_two" "$STATE_DIR/logs/connection-refresh-$client_two.log"
  client_two_hold_pid="$LAST_CLIENT_HOLD_PID"
  hold_connections_started=1
  wait_for_client_hold_connection "$client_one" "$STATE_DIR/logs/connection-refresh-$client_one.log" "$client_one_hold_pid"
  wait_for_client_hold_connection "$client_two" "$STATE_DIR/logs/connection-refresh-$client_two.log" "$client_two_hold_pid"
  wait_for_connection_ids "192.168.50.101" "$CONNECTION_REFRESH_TEST_HOST" \
    "$STATE_DIR/connection-refresh-before.json" "$STATE_DIR/connection-refresh-target.ids"
  wait_for_connection_ids "192.168.51.102" "$CONNECTION_REFRESH_TEST_HOST" \
    "$STATE_DIR/connection-refresh-before.json" "$STATE_DIR/connection-refresh-other.ids"

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_one" --slot default --policy DIRECT --format json >"$STATE_DIR/device-one-direct-for-refresh.json"
  /usr/bin/curl --fail --silent --show-error --request POST \
    --header "Authorization: Bearer $CONTROL_API_TOKEN" \
    "http://127.0.0.1:$CONTROL_API_PORT/api/v1/devices/$client_one/connections/refresh" \
    >"$STATE_DIR/connection-refresh-response.json"
  assert_connection_refresh_response "$STATE_DIR/connection-refresh-response.json" "$client_one"
  wait_for_scoped_connection_refresh "$STATE_DIR/connection-refresh-target.ids" \
    "$STATE_DIR/connection-refresh-other.ids" "$STATE_DIR/connection-refresh-after.json"

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_policy_log_for_host "device/$client_one/default" "DIRECT" "$host"
  assert_tun_egress_proxy_unused
  echo "device connection refresh removed only the target device old connection; its new connection used DIRECT"

  stop_client_hold_connection "$client_one_hold_pid"
  stop_client_hold_connection "$client_two_hold_pid"
  hold_connections_started=0

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy DIRECT --format json >"$STATE_DIR/device-two-direct-after-reload.json"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client udp "$LAN_IP" 1.1.1.1 443
  wait_for_tun_udp_reject "192.168.51.102" "1.1.1.1" 443

  cp "$STATE_DIR/device-policy.applied.json" "$STATE_DIR/device-policy.applied.evidence.json"
  cp "$STATE_DIR/state.json" "$STATE_DIR/state.evidence.json"
  restore_client_control_dns
  stop_control_api
  control_api_started=0
  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  stop_egress_probe
  egress_probe_started=0
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  stop_sudo_keepalive
  trap - EXIT INT TERM
  collect_artifacts
  echo "virtual LAN device-policy TUN test passed"
}

run_ipv6_userspace_test() {
  local client_one client_two source_one source_two gateway_started broker_pid iface rdnss client_gateway
  local egress_probe_started dns_fixture_started http3_probe_started udp_proxy_started topology config_mode
  topology="${1:-isolated_lan}"
  case "$topology" in
    isolated_lan) config_mode=ipv6 ;;
    same_wifi_dhcp) config_mode=ipv6-same-wifi ;;
    same_lan) config_mode=ipv6-same-lan ;;
    *) echo "unknown IPv6 lab topology: $topology" >&2; exit 2 ;;
  esac
  require_installed_lab
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_cached_sudo
  start_sudo_keepalive
  trap stop_sudo_keepalive EXIT
  ensure_lab_state_writable
  set -- $CLIENTS
  [[ "$#" -eq 2 ]] || { echo "IPv6 lab requires exactly two clients" >&2; exit 1; }
  client_one="$1"
  client_two="$2"
  iface="$(lab_interface)"

  rm -f "$STATE_DIR/cache.db" "$STATE_DIR/cache.db-journal" \
    "$STATE_DIR/ipv6-packet.sock" "$STATE_DIR/ipv6-packet.sock.broker" "$STATE_DIR/ipv6-packet.ready"
  rm -rf "$STATE_DIR/egress"
  # Do not carry ad-hoc tcpdump files from an earlier diagnostic session into
  # a new artifact bundle; managed gateway logs are recreated by start.
  rm -f "$STATE_DIR/logs/ipv6-packets-bridge102.log" "$STATE_DIR/logs/ipv6-packets-utun123.log"
  write_ipv6_device_policy_fixture "$topology"
  build_ipv6_lab_binaries
  build_egress_probe
  build_http3_lab_binaries
  install_http3_lab_client
  write_tun_egress_provider
  write_config "$config_mode"
  mkdir -p "$ROOT/bin"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$BINARY" ./cmd/omg

  gateway_started=0
  egress_probe_started=0
  dns_fixture_started=0
  http3_probe_started=0
  udp_proxy_started=0
  cleanup_test() {
    status=$?
    collect_artifacts || true
    restore_client_control_dns
    if [[ "$topology" == "same_lan" ]]; then
      for client in $CLIENTS; do
        limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client clear-manual || true
      done
    fi
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    if [[ "$udp_proxy_started" == 1 ]]; then
      stop_ipv6_udp_proxy || true
    fi
    if [[ "$http3_probe_started" == 1 ]]; then
      stop_http3_probe || true
    fi
    if [[ "$dns_fixture_started" == 1 ]]; then
      stop_ipv6_dns_fixture || true
    fi
    if [[ "$egress_probe_started" == 1 ]]; then
      stop_egress_probe || true
    fi
    stop_sudo_keepalive
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  start_egress_probe
  egress_probe_started=1
  start_http3_probe
  http3_probe_started=1
  start_ipv6_dns_fixture
  dns_fixture_started=1
  start_ipv6_udp_proxy
  udp_proxy_started=1
  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1
  rdnss="$(/sbin/ifconfig "$iface" | awk '/inet6 fe80:/ { split($2, value, "%"); print value[1]; exit }')"
  [[ "$rdnss" == fe80:* ]] || { echo "lab interface $iface has no link-local IPv6 gateway" >&2; exit 1; }
  if [[ "$topology" == "same_lan" ]]; then
    limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client manual "$LAN_IP" "192.168.50.101"
    limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client manual6 "$LAN_IP" "fdfe:dcba:9878::21" "$rdnss" "$rdnss"
    limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client manual "$LAN_IP" "192.168.50.102"
    limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client manual6 "$LAN_IP" "fdfe:dcba:9878::22" "$rdnss" "$rdnss"
    for client in $CLIENTS; do
      limactl shell "$client" -- grep -Fx "nameserver $rdnss" /etc/resolv.conf
    done
    grep -Fq 'listen-address=fdfe:dcba:9878::1' "$STATE_DIR/dnsmasq.conf"
    if grep -Eq '^(enable-ra|ra-param=|dhcp-option=option6:|dhcp-range=fdfe:)' "$STATE_DIR/dnsmasq.conf"; then
      echo "same-LAN selective IPv6 unexpectedly enabled router advertisements" >&2
      cat "$STATE_DIR/dnsmasq.conf" >&2
      exit 1
    fi
  else
    for client in $CLIENTS; do
      client_gateway="$LAN_IP"
      if [[ "$topology" == "same_wifi_dhcp" && "$client" == "$client_one" ]]; then
        client_gateway="$SAME_WIFI_BYPASS_GATEWAY"
      fi
      limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$client_gateway"
      limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew6 "$LAN_IP"
      limactl shell "$client" -- bash -lc "ip -6 route show default dev omg0 | grep -F 'pref medium'"
    done
    grep -F 'option: 23 dns-server' "$STATE_DIR/logs/dnsmasq.log" | grep -Fq "$rdnss" || {
      echo "dnsmasq did not advertise the link-local RDNSS address $rdnss" >&2
      tail -180 "$STATE_DIR/logs/dnsmasq.log" >&2 || true
      exit 1
    }
  fi
  assert_client_ipv4 "$client_one" "192.168.50.101"
  assert_client_ipv4 "$client_two" "192.168.50.102"
  if [[ "$topology" == "same_wifi_dhcp" ]]; then
    limactl shell "$client_one" -- bash -lc "ip -4 route show default dev omg0 | grep -F 'via $SAME_WIFI_BYPASS_GATEWAY'"
    limactl shell "$client_two" -- bash -lc "ip -4 route show default dev omg0 | grep -F 'via $LAN_IP'"
  fi
  source_one="$(client_ipv6 "$client_one")"
  source_two="$(client_ipv6 "$client_two")"
  [[ "$source_one" == fdfe:dcba:9878:* && "$source_two" == fdfe:dcba:9878:* && "$source_one" != "$source_two" ]] || {
    echo "clients did not receive distinct OpenSurge IPv6 addresses: $source_one $source_two" >&2
    exit 1
  }

  "$BINARY" status --config "$CONFIG" --format json >"$STATE_DIR/ipv6-status.json"
  grep -Fq '"dns_ipv6": true' "$STATE_DIR/ipv6-status.json"
  grep -Fq '"tun_ipv6_requested": "always"' "$STATE_DIR/ipv6-status.json"
  grep -Fq '"ipv6_packet": "ready"' "$STATE_DIR/ipv6-status.json"
  grep -Fq '"gateway": "running"' "$STATE_DIR/ipv6-status.json"
  if [[ "$topology" == "same_lan" ]]; then
    grep -Fq '"ipv6_ra_effective": false' "$STATE_DIR/state.json"
  else
    grep -Fq '"ipv6_ra_effective": true' "$STATE_DIR/state.json"
  fi
  grep -Fq 'type: opensurge-packet' "$STATE_DIR/mihomo.yaml"
  grep -Fq "\"$(client_mac "$client_one")\": \"device:$client_one\"" "$STATE_DIR/mihomo.yaml"
  grep -Fq "\"$(client_mac "$client_two")\": \"device:$client_two\"" "$STATE_DIR/mihomo.yaml"
  assert_out_of_lan_device_dormant
  if [[ "$topology" == "same_wifi_dhcp" ]]; then
    grep -Fq -- "- AND,((IN-TYPE,TUN),(IN-USER,device:$client_one)),REJECT" "$STATE_DIR/mihomo.yaml"
    "$BINARY" devices --config "$CONFIG" --format json >"$STATE_DIR/ipv6-devices.json"
    /usr/bin/ruby -rjson -e '
      device = JSON.parse(File.read(ARGV.fetch(0))).fetch("devices").find { |item| item["id"] == ARGV.fetch(1) }
      abort "upstream-router fixture device missing" unless device
      abort "upstream-router fixture was not preserved" unless device["gateway_target"] == "upstream_router"
      abort "upstream-router fixture did not report ipv6_blocked" unless device["ipv6_blocked"] == true
      abort "upstream-router fixture unexpectedly retained selectors" unless device.fetch("groups", {}).empty?
    ' "$STATE_DIR/ipv6-devices.json" "$client_one"
  fi

  if limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client ipv6-transparent "$LAN_IP" "$IPV6_TCP_TEST_URL"; then
    echo "$client_one unexpectedly bypassed its IPv6 REJECT rule" >&2
    exit 1
  fi
  wait_for_ipv6_policy_log TCP "$source_one" "$IPV6_TCP_TEST_HOST:$EGRESS_ORIGIN_PORT" REJECT

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client ipv6-transparent "$LAN_IP" "$IPV6_TCP_TEST_URL"
  wait_for_ipv6_policy_log TCP "$source_two" "$IPV6_TCP_TEST_HOST:$EGRESS_ORIGIN_PORT" "device/$client_two/default[DIRECT]"
  grep -Fq 'GET /ipv6-tcp' "$STATE_DIR/egress/origin.log"

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client ipv6-udp "$LAN_IP" "$IPV6_UDP_TEST_HOST" "$IPV6_DNS_FIXTURE_PORT" "$IPV6_UDP_ANSWER_HOST"
  wait_for_ipv6_policy_log UDP "$source_two" "$IPV6_UDP_TEST_HOST:$IPV6_DNS_FIXTURE_PORT" "device/$client_two/default[DIRECT]"

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client ipv6-quic "$LAN_IP" "$IPV6_QUIC_TEST_HOST" "$IPV6_DNS_FIXTURE_PORT"
  wait_for_ipv6_policy_log UDP "$source_two" "$IPV6_QUIC_TEST_HOST:$IPV6_DNS_FIXTURE_PORT" "device/$client_two/default[DIRECT]"

  run_ipv6_http3_client "$client_two" "$source_two" "$IPV6_HTTP3_DIRECT_HOST" "/ipv6-http3-direct" "http3-client-direct.txt"
  grep -Fq 'CLIENT_IPV6_HTTP3_OK protocol=HTTP/3.0' "$STATE_DIR/egress/http3-client-direct.txt"
  wait_for_ipv6_policy_log UDP "$source_two" "$IPV6_HTTP3_DIRECT_HOST:$IPV6_HTTP3_FIXTURE_PORT" "device/$client_two/default[DIRECT]"
  wait_for_http3_origin "$IPV6_HTTP3_DIRECT_HOST" "/ipv6-http3-direct"

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy lab-udp-proxy --format json \
    >"$STATE_DIR/egress/http3-select-udp-proxy.json"
  run_ipv6_http3_client "$client_two" "$source_two" "$IPV6_HTTP3_PROXY_HOST" "/ipv6-http3-proxy" "http3-client-udp-proxy.txt"
  grep -Fq 'CLIENT_IPV6_HTTP3_OK protocol=HTTP/3.0' "$STATE_DIR/egress/http3-client-udp-proxy.txt"
  wait_for_ipv6_policy_log UDP "$source_two" "$IPV6_HTTP3_PROXY_HOST:$IPV6_HTTP3_FIXTURE_PORT" "device/$client_two/default[lab-udp-proxy]"
  wait_for_http3_origin "$IPV6_HTTP3_PROXY_HOST" "/ipv6-http3-proxy"
  wait_for_controlled_udp_proxy

  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy lab-http-only --format json \
    >"$STATE_DIR/egress/http3-select-http-only.json"
  if run_ipv6_http3_client "$client_two" "$source_two" "$IPV6_HTTP3_BLOCKED_HOST" "/ipv6-http3-blocked" "http3-client-http-only.txt"; then
    echo "HTTP/3 unexpectedly crossed the HTTP-only outbound" >&2
    exit 1
  fi
  wait_for_ipv6_policy_log UDP "$source_two" "$IPV6_HTTP3_BLOCKED_HOST:$IPV6_HTTP3_FIXTURE_PORT" REJECT
  if grep -Fq '/ipv6-http3-blocked' "$STATE_DIR/egress/http3-origin.log"; then
    echo "HTTP-only fail-closed probe unexpectedly reached the HTTP/3 origin" >&2
    exit 1
  fi
  if [[ -s "$STATE_DIR/egress/proxy.log" ]]; then
    echo "HTTP-only fail-closed probe unexpectedly reached the controlled CONNECT proxy" >&2
    cat "$STATE_DIR/egress/proxy.log" >&2
    exit 1
  fi
  "$BINARY" device-policy-select --config "$CONFIG" --device "$client_two" --slot default --policy DIRECT --format json \
    >"$STATE_DIR/egress/http3-select-direct-restored.json"
  assert_ipv6_bpf_bidirectional

  cp "$STATE_DIR/state.json" "$STATE_DIR/ipv6-state.evidence.json"
  broker_pid="$(sed -n 's/.*"pid_ipv6_packet": \([0-9][0-9]*\).*/\1/p' "$STATE_DIR/state.json" | head -1)"
  [[ -n "$broker_pid" ]] || { echo "IPv6 packet broker PID missing from runtime state" >&2; exit 1; }
  /sbin/ifconfig "$iface" | grep -Fq 'inet6 fdfe:dcba:9878::1'

  restore_client_control_dns
  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  stop_ipv6_udp_proxy
  udp_proxy_started=0
  stop_http3_probe
  http3_probe_started=0
  stop_ipv6_dns_fixture
  dns_fixture_started=0
  stop_egress_probe
  egress_probe_started=0
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  for path in "$STATE_DIR/ipv6-packet.sock" "$STATE_DIR/ipv6-packet.sock.broker" "$STATE_DIR/ipv6-packet.ready"; do
    [[ ! -e "$path" ]] || { echo "IPv6 runtime path remained after stop: $path" >&2; exit 1; }
  done
  if /bin/kill -0 "$broker_pid" 2>/dev/null; then
    echo "IPv6 packet broker pid $broker_pid remained alive after stop" >&2
    exit 1
  fi
  if /sbin/ifconfig "$iface" | grep -Fq 'inet6 fdfe:dcba:9878::1'; then
    echo "OpenSurge IPv6 gateway alias remained on $iface after stop" >&2
    exit 1
  fi
  if [[ "$topology" == "same_lan" ]]; then
    for client in $CLIENTS; do
      limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client clear-manual
    done
  else
    for client in $CLIENTS; do
      assert_client_ipv6_withdrawn "$client"
    done
  fi
  stop_sudo_keepalive
  trap - EXIT INT TERM
  collect_artifacts
  echo "virtual LAN $topology userspace IPv6 TCP, UDP, QUIC carrier, HTTP/3-only DIRECT and SOCKS5 UDP, HTTP-only fail-closed, device identity, upstream-router IPv6 block, and rollback test passed"
}

run_ipv6_imported_egress_test() {
  local client_one client_two source_one source_two gateway_started broker_pid iface rdnss
  local direct_native_exit vm_direct_exit cleanup_complete status
  require_installed_lab
  require_command curl
  require_command python3
  require_command ruby
  [[ -r "$INTERFACE_FILE" ]] || { echo "lab is not up; run: make lab-up" >&2; exit 1; }
  require_cached_sudo
  ensure_lab_state_writable
  set -- $CLIENTS
  [[ "$#" -eq 2 ]] || { echo "IPv6 lab requires exactly two clients" >&2; exit 1; }
  client_one="$1"
  client_two="$2"
  iface="$(lab_interface)"

  gateway_started=0
  cleanup_complete=1
  cleanup_test() {
    status=$?
    trap - EXIT INT TERM
    restore_client_control_dns
    if [[ "$gateway_started" == 1 ]]; then
      if sudo -n "$BINARY" stop --config "$CONFIG"; then
        gateway_started=0
      else
        cleanup_complete=0
        status=1
      fi
    fi
    stop_sudo_keepalive
    collect_ipv6_real_artifacts || true
    if [[ -n "$LAST_LAB_ARTIFACT_DIR" && -f "$IPV6_REAL_PROFILE" ]]; then
      assert_ipv6_real_artifacts_safe "$LAST_LAB_ARTIFACT_DIR" "$IPV6_REAL_PROFILE" || status=1
    fi
    if [[ "$cleanup_complete" == 1 ]]; then
      cleanup_ipv6_real_secrets
    else
      echo "gateway cleanup was incomplete; the private runtime profile was retained for recovery" >&2
    fi
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM
  start_sudo_keepalive

  rm -f "$STATE_DIR/cache.db" "$STATE_DIR/cache.db-journal" \
    "$STATE_DIR/ipv6-packet.sock" "$STATE_DIR/ipv6-packet.sock.broker" "$STATE_DIR/ipv6-packet.ready" \
    "$STATE_DIR/ipv6-real-status.json" "$STATE_DIR/ipv6-real-egress.txt"
  rm -f "$STATE_DIR/logs/ipv6-packets-bridge102.log" "$STATE_DIR/logs/ipv6-packets-utun123.log"
  write_ipv6_real_profile_and_policy
  build_ipv6_lab_binaries
  write_config ipv6-auto
  mkdir -p "$ROOT/bin"
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$BINARY" ./cmd/omg

  direct_native_exit="$(native_ipv6_exit_address)"
  [[ "$direct_native_exit" == *:* ]] || {
    echo "the selected upstream interface did not provide a direct native IPv6 HTTPS exit" >&2
    exit 1
  }
  assert_native_ipv6_exit_on_upstream "$direct_native_exit"
  echo "native IPv6 upstream HTTPS preflight passed"

  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1
  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew6 "$LAN_IP"
  done

  rdnss="$(/sbin/ifconfig "$iface" | awk '/inet6 fe80:/ { split($2, value, "%"); print value[1]; exit }')"
  [[ "$rdnss" == fe80:* ]] || { echo "lab interface $iface has no link-local RDNSS address" >&2; exit 1; }
  grep -F 'option: 23 dns-server' "$STATE_DIR/logs/dnsmasq.log" | grep -Fq "$rdnss" || {
    echo "dnsmasq did not advertise its link-local RDNSS address" >&2
    exit 1
  }
  assert_client_ipv4 "$client_one" "192.168.50.101"
  assert_client_ipv4 "$client_two" "192.168.50.102"
  source_one="$(client_ipv6 "$client_one")"
  source_two="$(client_ipv6 "$client_two")"
  [[ "$source_one" == fdfe:dcba:9878:* && "$source_two" == fdfe:dcba:9878:* && "$source_one" != "$source_two" ]] || {
    echo "clients did not receive distinct OpenSurge IPv6 addresses" >&2
    exit 1
  }

  "$BINARY" status --config "$CONFIG" --format json >"$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"dns_ipv6": true' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"tun_ipv6_requested": "auto"' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"native_ipv6_available": true' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"ipv6_reason": "native_ipv6_available"' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"ipv6_packet": "ready"' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq '"gateway": "running"' "$STATE_DIR/ipv6-real-status.json"
  grep -Fq 'type: opensurge-packet' "$STATE_DIR/mihomo.yaml"
  grep -Fq "\"$(client_mac "$client_one")\": \"device:$client_one\"" "$STATE_DIR/mihomo.yaml"
  grep -Fq "\"$(client_mac "$client_two")\": \"device:$client_two\"" "$STATE_DIR/mihomo.yaml"
  grep -F -- "IN-USER,device:$client_one" "$STATE_DIR/mihomo.yaml" |
    grep -F -- "$IPV6_NATIVE_DIRECT_HOST" | grep -Fq -- 'DIRECT'
  grep -F -- "IN-USER,device:$client_one" "$STATE_DIR/mihomo.yaml" |
    grep -F -- 'NETWORK,udp' | grep -F -- 'DST-PORT,53' | grep -Fq -- 'DIRECT'
  grep -F -- "IN-USER,device:$client_one" "$STATE_DIR/mihomo.yaml" |
    grep -F -- 'NETWORK,udp' | grep -F -- 'DST-PORT,443' | grep -Fq -- 'DIRECT'

  /usr/bin/curl --fail --silent --show-error --request PATCH \
    --header 'Content-Type: application/json' \
    --data '{"mode":"rule","log-level":"info"}' \
    http://127.0.0.1:19090/configs >/dev/null

  if limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client ipv6-transparent "$LAN_IP" "$IPV6_REAL_TCP_URL"; then
    echo "$client_one unexpectedly bypassed its real-profile IPv6 REJECT rule" >&2
    exit 1
  fi
  wait_for_ipv6_policy_log TCP "$source_one" "$IPV6_REAL_TCP_HOST:443" REJECT

  vm_direct_exit="$(limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-native-https-exit "$LAN_IP" "$IPV6_NATIVE_DIRECT_URL" | tr -d '\r\n')"
  assert_native_ipv6_exit_on_upstream "$vm_direct_exit"
  wait_for_ipv6_direct_log TCP "$source_one" "$IPV6_NATIVE_DIRECT_HOST" 443

  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-public-udp "$LAN_IP" "$IPV6_NATIVE_DIRECT_TARGET" "$IPV6_REAL_UDP_QUERY"
  wait_for_ipv6_direct_log UDP "$source_one" "$IPV6_NATIVE_DIRECT_TARGET" 53

  limactl shell "$client_one" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-public-quic "$LAN_IP" "$IPV6_NATIVE_DIRECT_TARGET"
  wait_for_ipv6_direct_log UDP "$source_one" "$IPV6_NATIVE_DIRECT_TARGET" 443

  select_working_real_proxy "$direct_native_exit"
  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client ipv6-transparent "$LAN_IP" "$IPV6_REAL_TCP_URL"
  wait_for_ipv6_global_log TCP "$source_two" "$IPV6_REAL_TCP_HOST" 443

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-public-https "$LAN_IP" "$IPV6_REAL_TARGET" "$IPV6_REAL_TARGET_URL"
  wait_for_ipv6_global_log TCP "$source_two" "$IPV6_REAL_TARGET" 443

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-public-udp "$LAN_IP" "$IPV6_REAL_UDP_TARGET" "$IPV6_REAL_UDP_QUERY"
  wait_for_ipv6_global_log UDP "$source_two" "$IPV6_REAL_UDP_TARGET" 53

  limactl shell "$client_two" -- sudo /usr/local/bin/omg-lab-client \
    ipv6-public-quic "$LAN_IP" "$IPV6_REAL_UDP_TARGET"
  wait_for_ipv6_global_log UDP "$source_two" "$IPV6_REAL_UDP_TARGET" 443

  broker_pid="$(sed -n 's/.*"pid_ipv6_packet": \([0-9][0-9]*\).*/\1/p' "$STATE_DIR/state.json" | head -1)"
  [[ -n "$broker_pid" ]] || { echo "IPv6 packet broker PID missing from runtime state" >&2; exit 1; }
  /sbin/ifconfig "$iface" | grep -Fq 'inet6 fdfe:dcba:9878::1'
  {
    echo "native_ipv6_auto=passed"
    echo "subscription_import=passed"
    echo "device_identity_reject=passed"
    echo "vm_native_direct_https=passed"
    echo "vm_native_direct_exit_is_mac_gua=passed"
    echo "vm_native_direct_udp=passed"
    echo "vm_native_direct_quic_carrier=passed"
    echo "selected_candidate_index=$REAL_PROXY_INDEX"
    echo "selected_proxy_type=$REAL_PROXY_TYPE"
    echo "selected_proxy_exit_family=$REAL_PROXY_EXIT_FAMILY"
    echo "vm_fake_aaaa_https=passed"
    echo "vm_public_ipv6_https=passed"
    echo "vm_public_ipv6_udp=passed"
    echo "vm_public_ipv6_quic_carrier=passed"
  } >"$STATE_DIR/ipv6-real-egress.txt"

  restore_client_control_dns
  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  stop_sudo_keepalive
  [[ ! -e "$STATE_DIR/state.json" ]] || { echo "gateway state was not removed" >&2; exit 1; }
  for path in "$STATE_DIR/ipv6-packet.sock" "$STATE_DIR/ipv6-packet.sock.broker" "$STATE_DIR/ipv6-packet.ready"; do
    [[ ! -e "$path" ]] || { echo "IPv6 runtime path remained after stop: $path" >&2; exit 1; }
  done
  if /bin/kill -0 "$broker_pid" 2>/dev/null; then
    echo "IPv6 packet broker remained alive after stop" >&2
    exit 1
  fi
  if /sbin/ifconfig "$iface" | grep -Fq 'inet6 fdfe:dcba:9878::1'; then
    echo "OpenSurge IPv6 gateway alias remained after stop" >&2
    exit 1
  fi
  for client in $CLIENTS; do
    assert_client_ipv6_withdrawn "$client"
  done

  collect_ipv6_real_artifacts
  assert_ipv6_real_artifacts_safe "$LAST_LAB_ARTIFACT_DIR" "$IPV6_REAL_PROFILE"
  cleanup_ipv6_real_secrets
  trap - EXIT INT TERM
  echo "virtual LAN native IPv6 DIRECT, real subscription GLOBAL, identity, and rollback test passed"
}

run_local_routing_assertions() {
  local client client_ip host first_line
  set -- $CLIENTS
  [[ "$#" -ge 1 ]] || { echo "local-routing lab requires at least one client" >&2; exit 1; }
  client="$1"
  client_ip="$(client_ipv4 "$client")"
  [[ -n "$client_ip" ]] || { echo "could not resolve $client IPv4" >&2; exit 1; }
  host="$(url_host "$TEST_URL")"

  wait_for_policy_option TunEgress egress-proxy

  "$BINARY" policy-select --config "$CONFIG" --group TunEgress --policy DIRECT --format json >"$STATE_DIR/local-routing-gateway-direct.json"
  "$BINARY" local-routing-set --config "$CONFIG" --mode rule --format json >"$STATE_DIR/local-routing-rule.json"
  grep -Fq '"mode": "rule"' "$STATE_DIR/local-routing-rule.json"

  : >"$STATE_DIR/egress/proxy.log"
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  curl --noproxy '*' --fail --silent --show-error --max-time 15 "$TEST_URL" >"$STATE_DIR/local-routing-rule-host.out"
  wait_for_tun_source_log_after "$host" "198.18.0.1" "$first_line"
  wait_for_tun_policy_log_for_host TunEgress DIRECT "$host"
  assert_tun_egress_proxy_unused
  run_local_routing_ipv6_tcp rule TunEgress
  run_local_routing_ipv6_http3 rule 'TunEgress[DIRECT]'

  "$BINARY" local-routing-set --config "$CONFIG" --mode global --policy egress-proxy --format json >"$STATE_DIR/local-routing-global.json"
  grep -Fq '"mode": "global"' "$STATE_DIR/local-routing-global.json"
  grep -Fq '"udp_behavior": "reject"' "$STATE_DIR/local-routing-global.json"
  : >"$STATE_DIR/egress/proxy.log"
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  curl --noproxy '*' --fail --silent --show-error --max-time 15 "$TEST_URL" >"$STATE_DIR/local-routing-global-host.out"
  wait_for_tun_source_log_after "$host" "198.18.0.1" "$first_line"
  assert_tun_egress_proxy_used
  run_local_routing_ipv6_tcp global open-surge/mac-mode-tcp
  run_local_routing_ipv6_http3 global 'open-surge/mac-mode-udp[REJECT]'

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_action_log "$host" "TunEgress[DIRECT]" "$client_ip"
  assert_tun_egress_proxy_unused

  "$BINARY" policy-select --config "$CONFIG" --group TunEgress --policy egress-proxy --format json >"$STATE_DIR/local-routing-gateway-proxy.json"
  "$BINARY" local-routing-set --config "$CONFIG" --mode direct --format json >"$STATE_DIR/local-routing-direct.json"
  grep -Fq '"mode": "direct"' "$STATE_DIR/local-routing-direct.json"
  : >"$STATE_DIR/egress/proxy.log"
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  curl --noproxy '*' --fail --silent --show-error --max-time 15 "$TEST_URL" >"$STATE_DIR/local-routing-direct-host.out"
  wait_for_tun_source_log_after "$host" "198.18.0.1" "$first_line"
  assert_tun_egress_proxy_unused
  run_local_routing_ipv6_tcp direct open-surge/mac-mode-tcp
  run_local_routing_ipv6_http3 direct 'open-surge/mac-mode-udp[DIRECT]'

  : >"$STATE_DIR/egress/proxy.log"
  limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client transparent "$LAN_IP" "$TEST_URL"
  wait_for_tun_action_log "$host" "TunEgress[egress-proxy]" "$client_ip"
  assert_tun_egress_proxy_used

  "$BINARY" local-routing-set --config "$CONFIG" --mode rule --format json >"$STATE_DIR/local-routing-rule-follow.json"
  : >"$STATE_DIR/egress/proxy.log"
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  curl --noproxy '*' --fail --silent --show-error --max-time 15 "$TEST_URL" >"$STATE_DIR/local-routing-rule-follow-host.out"
  wait_for_tun_source_log_after "$host" "198.18.0.1" "$first_line"
  assert_tun_egress_proxy_used

  "$BINARY" policies --config "$CONFIG" --format json >"$STATE_DIR/local-routing-visible-policies.json"
  if grep -Fq '"name": "open-surge/mac-' "$STATE_DIR/local-routing-visible-policies.json"; then
    echo "generic policies exposed local-routing internal groups" >&2
    cat "$STATE_DIR/local-routing-visible-policies.json" >&2
    exit 1
  fi
  echo "local Mac routing isolation test passed"
}

run_local_routing_ipv6_tcp() {
  local mode=$1 action=$2 source_ip fake_ip first_line output
  source_ip="fdfe:dcba:9876::1"
  output="$STATE_DIR/local-routing-$mode-ipv6-tcp.out"
  fake_ip="$(dig +time=5 +tries=1 +short @127.0.0.1 -p "$CONFIG_DNS_PORT" "$LOCAL_ROUTING_IPV6_HTTP3_HOST" AAAA | awk '/^fdfe:dcba:9876:/ { print; exit }')"
  [[ -n "$fake_ip" ]] || { echo "local-routing Lab did not receive a fake-AAAA address" >&2; exit 1; }
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  if curl --noproxy '*' --ipv6 --interface "$source_ip" \
    --resolve "$LOCAL_ROUTING_IPV6_HTTP3_HOST:443:[$fake_ip]" \
    --connect-timeout 3 --max-time 5 --fail --silent --show-error \
    "https://$LOCAL_ROUTING_IPV6_HTTP3_HOST/" >"$output" 2>&1; then
    echo "local IPv6 TCP unexpectedly reached the TEST-NET-1 fixture target in $mode mode" >&2
    exit 1
  fi
  wait_for_local_ipv6_log_after TCP "$source_ip" "$LOCAL_ROUTING_IPV6_HTTP3_HOST" 443 "$action" "$first_line"
}

run_local_routing_ipv6_http3() {
  local mode=$1 action=$2 source_ip fake_ip first_line request_path origin_log output
  source_ip="fdfe:dcba:9876::1"
  request_path="/local-mac-$mode"
  origin_log="$STATE_DIR/egress/http3-origin.log"
  output="$STATE_DIR/local-routing-$mode-ipv6-http3.out"
  fake_ip="$(dig +time=5 +tries=1 +short @127.0.0.1 -p "$CONFIG_DNS_PORT" "$LOCAL_ROUTING_IPV6_HTTP3_HOST" AAAA | awk '/^fdfe:dcba:9876:/ { print; exit }')"
  [[ -n "$fake_ip" ]] || { echo "local-routing Lab did not receive a fake-AAAA address" >&2; exit 1; }
  : >"$origin_log"
  first_line="$(( $(wc -l <"$STATE_DIR/logs/mihomo.log") + 1 ))"
  # The fixture maps the host to TEST-NET-1 so the generated private-destination
  # bypass rules cannot mask the local-mode action under test. The QUIC request
  # is expected to fail after mihomo records the selected action.
  if "$HTTP3_PROBE_BINARY" client \
    --url "https://$LOCAL_ROUTING_IPV6_HTTP3_HOST:$IPV6_HTTP3_FIXTURE_PORT$request_path" \
    --address "$fake_ip" --source "$source_ip" >"$output" 2>&1; then
    echo "local IPv6 HTTP/3 unexpectedly reached the TEST-NET-1 fixture target in $mode mode" >&2
    exit 1
  fi
  wait_for_local_ipv6_log_after UDP "$source_ip" "$LOCAL_ROUTING_IPV6_HTTP3_HOST" "$IPV6_HTTP3_FIXTURE_PORT" "$action" "$first_line"
  [[ ! -s "$origin_log" ]] || { echo "local IPv6 HTTP/3 unexpectedly reached the origin" >&2; cat "$origin_log" >&2; exit 1; }
}

run_test() {
  local mode client gateway_started egress_probe_started dns_fixture_started http3_probe_started
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
  dns_fixture_started=0
  http3_probe_started=0
  cleanup_test() {
    status=$?
    collect_artifacts || true
    restore_client_control_dns
    if [[ "$gateway_started" == 1 ]]; then
      sudo -n "$BINARY" stop --config "$CONFIG" || true
    fi
    if [[ "$egress_probe_started" == 1 ]]; then
      stop_egress_probe || true
    fi
    if [[ "$http3_probe_started" == 1 ]]; then
      stop_http3_probe || true
    fi
    if [[ "$dns_fixture_started" == 1 ]]; then
      stop_ipv6_dns_fixture || true
    fi
    exit "$status"
  }
  trap cleanup_test EXIT INT TERM

  if [[ "$TUN_EGRESS_PROFILE" == 1 ]]; then
    build_egress_probe
    start_egress_probe
    egress_probe_started=1
  fi
  if [[ "$LOCAL_ROUTING_TEST" == "true" ]]; then
    build_http3_lab_binaries
    start_http3_probe
    http3_probe_started=1
    start_ipv6_dns_fixture
    dns_fixture_started=1
  fi

  sudo -n "$BINARY" start --config "$CONFIG"
  gateway_started=1

  if [[ "$LOCAL_ROUTING_TEST" == "true" ]]; then
    grep -Fq 'fake-ip-range6: fdfe:dcba:9876::/64' "$STATE_DIR/mihomo.yaml"
    grep -Fq 'AND,((IN-TYPE,TUN),(IN-NAME,DEFAULT-TUN),(SRC-IP-CIDR,fdfe:dcba:9876::1/128),(NETWORK,TCP)),open-surge/mac-mode-tcp' "$STATE_DIR/mihomo.yaml"
    grep -Fq 'AND,((IN-TYPE,TUN),(IN-NAME,DEFAULT-TUN),(SRC-IP-CIDR,fdfe:dcba:9876::1/128),(NETWORK,UDP)),open-surge/mac-mode-udp' "$STATE_DIR/mihomo.yaml"
    if grep -F 'open-surge/mac-mode-' "$STATE_DIR/mihomo.yaml" | grep -Eq 'fdfe:dcba:9876::/64|fdfe:dcba:9878::/64|fc00::/7|IN-NAME,opensurge-ipv6'; then
      echo "local Mac routing rules captured a broad or downstream IPv6 identity" >&2
      exit 1
    fi
  fi

  for client in $CLIENTS; do
    limactl shell "$client" -- sudo /usr/local/bin/omg-lab-client renew "$LAN_IP"
  done

  if [[ "$LOCAL_ROUTING_TEST" == "true" ]]; then
    if [[ "$mode" != "tun" || "$TUN_EGRESS_PROFILE" != 1 ]]; then
      echo "local-routing lab requires TUN and the imported egress fixture" >&2
      exit 1
    fi
    run_local_routing_assertions
  elif [[ "$mode" == "tun" && "$TUN_EGRESS_PROFILE" == 1 ]]; then
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

  restore_client_control_dns
  sudo -n "$BINARY" stop --config "$CONFIG"
  gateway_started=0
  if [[ "$egress_probe_started" == 1 ]]; then
    stop_egress_probe
    egress_probe_started=0
  fi
  if [[ "$http3_probe_started" == 1 ]]; then
    stop_http3_probe
    http3_probe_started=0
  fi
  if [[ "$dns_fixture_started" == 1 ]]; then
    stop_ipv6_dns_fixture
    dns_fixture_started=0
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
  test-ipv6-userspace)
    run_ipv6_userspace_test isolated_lan
    ;;
  test-ipv6-same-wifi)
    run_ipv6_userspace_test same_wifi_dhcp
    ;;
  test-ipv6-same-lan)
    run_ipv6_userspace_test same_lan
    ;;
  test-ipv6-imported-egress)
    run_ipv6_imported_egress_test
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
    echo "usage: $0 <check|up|status|test|test-tun|test-tun-device-policy|test-ipv6-userspace|test-ipv6-same-wifi|test-ipv6-same-lan|test-ipv6-imported-egress|down|destroy>" >&2
    exit 2
    ;;
esac
