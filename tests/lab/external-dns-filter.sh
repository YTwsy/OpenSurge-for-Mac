# Optional controlled filter assertions shared by the existing network gates.
# Sourced by lab.sh; no resources are acquired until a gate starts.
EXTERNAL_DNS_FILTER_TEST="${OMG_LAB_EXTERNAL_DNS_FILTER:-false}"
EXTERNAL_DNS_FILTER_PORT="${OMG_LAB_EXTERNAL_DNS_FILTER_PORT:-19100}"
EXTERNAL_DNS_FILTER_PID=""
EXTERNAL_DNS_FILTER_EVIDENCE_STARTED=false

write_external_dns_filter_config() {
  [[ "$EXTERNAL_DNS_FILTER_TEST" == true ]] || return 0
  cat >>"$CONFIG" <<EOF

external_dns_filter:
  enabled: true
  server: "127.0.0.1:$EXTERNAL_DNS_FILTER_PORT"
  timeout_ms: 300
  blocked_addresses: "192.0.2.99"
  bypass_domains: ""
EOF
}

start_external_dns_filter() {
  [[ "$EXTERNAL_DNS_FILTER_TEST" == true ]] || return 0
  mkdir -p "$STATE_DIR/logs"
  if [[ "$EXTERNAL_DNS_FILTER_EVIDENCE_STARTED" == false ]]; then
    : >"$STATE_DIR/external-dns-filter-evidence.txt"
    EXTERNAL_DNS_FILTER_EVIDENCE_STARTED=true
  fi
  GOCACHE="${GOCACHE:-/private/tmp/open-mihomo-gateway-go-cache}" \
    go build -o "$STATE_DIR/dns-filter-probe" ./tests/integration/dnsfilterprobe
  printf 'allow\n' >"$STATE_DIR/dns-filter-decision"
  "$STATE_DIR/dns-filter-probe" --listen "127.0.0.1:$EXTERNAL_DNS_FILTER_PORT" \
    --control "$STATE_DIR/dns-filter-decision" >"$STATE_DIR/logs/dns-filter-fixture.log" 2>&1 &
  EXTERNAL_DNS_FILTER_PID=$!
  for i in {1..50}; do
    if grep -q '^DNS_FILTER_READY' "$STATE_DIR/logs/dns-filter-fixture.log"; then return 0; fi
    kill -0 "$EXTERNAL_DNS_FILTER_PID" 2>/dev/null || { cat "$STATE_DIR/logs/dns-filter-fixture.log" >&2; return 1; }
    sleep 0.1
  done
  echo "external DNS filter fixture did not become ready" >&2
  return 1
}

stop_external_dns_filter() {
  [[ -n "$EXTERNAL_DNS_FILTER_PID" ]] || return 0
  kill "$EXTERNAL_DNS_FILTER_PID" 2>/dev/null || true
  wait "$EXTERNAL_DNS_FILTER_PID" 2>/dev/null || true
  EXTERNAL_DNS_FILTER_PID=""
}

external_filter_client_query() {
  local client=$1 endpoint=$2 transport=$3 name=$4 type=$5 rcode=$6 answer=${7:-}
  limactl shell "$client" -- bash -s -- "$endpoint" "$transport" "$name" "$type" "$rcode" "$answer" <<'PROBE'
set -euo pipefail
endpoint=$1 transport=$2 name=$3 type=$4 rcode=$5 answer=$6
flags=()
[[ "$transport" != tcp ]] || flags+=(+tcp)
output=$(dig +time=3 +tries=1 +noall +comments +answer "${flags[@]}" "@$endpoint" "$name" "$type")
grep -q "status: $rcode," <<<"$output" || { echo "$output" >&2; exit 1; }
if [[ -n "$answer" ]]; then
  awk -v expected="$answer" '$0 !~ /^;/ && $NF ~ expected { found=1 } END { exit !found }' <<<"$output" || { echo "$output" >&2; exit 1; }
elif [[ "$rcode" == NOERROR ]]; then
  grep -q 'ANSWER: 0,' <<<"$output" || { echo "$output" >&2; exit 1; }
fi
echo "DNS_FILTER_CLIENT_OK $transport $type $name $rcode"
PROBE
}

assert_external_dns_filter() {
  [[ "$EXTERNAL_DNS_FILTER_TEST" == true ]] || return 0
  local client transport ipv6=false
  local -a flags
  grep -Fxq 'cache-size=0' "$STATE_DIR/dnsmasq.conf"
  grep -q 'type: opensurge-packet' "$STATE_DIR/mihomo.yaml" && ipv6=true
  : >"$STATE_DIR/external-dns-filter-evidence.txt"
  for client in $CLIENTS; do
    for transport in udp tcp; do
      external_filter_client_query "$client" "$LAN_IP" "$transport" allowed.opensurge.test A NOERROR '^198\.18\.'
      external_filter_client_query "$client" "$LAN_IP" "$transport" blocked.opensurge.test A NXDOMAIN
      external_filter_client_query "$client" "$LAN_IP" "$transport" blocked.opensurge.test AAAA NXDOMAIN
      external_filter_client_query "$client" "$LAN_IP" "$transport" refused.opensurge.test A REFUSED
      external_filter_client_query "$client" "$LAN_IP" "$transport" sink.opensurge.test A NOERROR '^0\.0\.0\.0$'
      external_filter_client_query "$client" "$LAN_IP" "$transport" custom.opensurge.test A NOERROR '^192\.0\.2\.99$'
      external_filter_client_query "$client" "$LAN_IP" "$transport" empty.opensurge.test A NOERROR
      external_filter_client_query "$client" "$LAN_IP" "$transport" truncated.opensurge.test A REFUSED
      external_filter_client_query "$client" "$LAN_IP" "$transport" timeout.opensurge.test A SERVFAIL
      if [[ "$ipv6" == true ]]; then
        external_filter_client_query "$client" fdfe:dcba:9878::1 "$transport" allowed.opensurge.test AAAA NOERROR '^fdfe:dcba:9876:'
        external_filter_client_query "$client" fdfe:dcba:9878::1 "$transport" blocked.opensurge.test AAAA NXDOMAIN
        external_filter_client_query "$client" fdfe:dcba:9878::1 "$transport" sink.opensurge.test AAAA NOERROR '^::$'
      fi
    done
  done >>"$STATE_DIR/external-dns-filter-evidence.txt"
  # Same name, before TTL expiry: neither Fake-IP persistence nor dnsmasq may
  # preserve a prior allow after the filter starts blocking it.
  for client in $CLIENTS; do
    external_filter_client_query "$client" "$LAN_IP" udp flip.opensurge.test A NOERROR '^198\.18\.'
  done >>"$STATE_DIR/external-dns-filter-evidence.txt"
  printf 'block\n' >"$STATE_DIR/dns-filter-decision"
  for client in $CLIENTS; do
    external_filter_client_query "$client" "$LAN_IP" tcp flip.opensurge.test A NXDOMAIN
  done >>"$STATE_DIR/external-dns-filter-evidence.txt"
  if grep -q '^  mode: "tun"' "$CONFIG" && grep -A1 '^local_system_dns:' "$CONFIG" | grep -q 'true'; then
    for transport in udp tcp; do
      flags=(); [[ "$transport" != tcp ]] || flags+=(+tcp)
      dig +time=3 +tries=1 +noall +comments "${flags[@]}" @114.114.114.114 blocked.mac.opensurge.test A | grep -q 'status: NXDOMAIN,'
    done
    /usr/bin/ruby -rsocket -rsecurerandom -e '
      nonce=SecureRandom.hex(6)
      addresses=Addrinfo.getaddrinfo("allowed.#{nonce}.opensurge.test",nil,Socket::AF_INET,Socket::SOCK_STREAM).map(&:ip_address)
      abort "system DNS did not return Fake-IP" unless addresses.any?{|ip|ip.start_with?("198.18.")}
      begin
        Addrinfo.getaddrinfo("blocked.#{nonce}.opensurge.test",nil,Socket::AF_INET,Socket::SOCK_STREAM)
        abort "system DNS bypassed filter"
      rescue SocketError
        puts "DNS_FILTER_MAC_SYSTEM_OK"
      end
    ' >>"$STATE_DIR/external-dns-filter-evidence.txt"
  fi
  stop_external_dns_filter
  for client in $CLIENTS; do
    external_filter_client_query "$client" "$LAN_IP" udp allowed.opensurge.test A SERVFAIL
  done >>"$STATE_DIR/external-dns-filter-evidence.txt"
  start_external_dns_filter
  for client in $CLIENTS; do
    external_filter_client_query "$client" "$LAN_IP" tcp allowed.opensurge.test A NOERROR '^198\.18\.'
  done >>"$STATE_DIR/external-dns-filter-evidence.txt"
  cat "$STATE_DIR/external-dns-filter-evidence.txt"
}
