#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

BASE_DIR="$ROOT/runtime/integration/policy-control"
WORK_DIR="${OMG_POLICY_CONTROL_WORK_DIR:-$BASE_DIR/run-$$}"
CONFIG="$WORK_DIR/config.yaml"
PROFILE="$WORK_DIR/profile.yaml"
PROVIDER="$WORK_DIR/provider.yaml"
REMOTE_PROVIDER="$WORK_DIR/remote-provider.yaml"
DEVICE_POLICY="$WORK_DIR/device-policy.json"
MIHOMO_CONFIG="$WORK_DIR/mihomo.yaml"
MIHOMO_LOG="$WORK_DIR/logs/mihomo.log"
OMG_BIN="$WORK_DIR/omg"
EGRESS_PROBE_BIN="$WORK_DIR/egress-probe"
MIHOMO_BINARY="${OMG_POLICY_CONTROL_MIHOMO_BINARY:-$ROOT/runtime/tools/bin/mihomo}"
API_ADDR="${OMG_POLICY_CONTROL_API_ADDR:-127.0.0.1:19091}"
MIXED_PORT="${OMG_POLICY_CONTROL_MIXED_PORT:-19092}"
EGRESS_ORIGIN_PORT="${OMG_POLICY_CONTROL_EGRESS_ORIGIN_PORT:-19093}"
EGRESS_PROXY_PORT="${OMG_POLICY_CONTROL_EGRESS_PROXY_PORT:-19094}"
PID=""
EGRESS_PROBE_PID=""

section() {
  printf '\n== %s ==\n' "$1"
}

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  if [[ -n "$EGRESS_PROBE_PID" ]] && kill -0 "$EGRESS_PROBE_PID" 2>/dev/null; then
    kill "$EGRESS_PROBE_PID" 2>/dev/null || true
    wait "$EGRESS_PROBE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

require_file() {
  if [[ ! -x "$1" ]]; then
    echo "required executable not found: $1" >&2
    exit 1
  fi
}

write_fixture() {
  mkdir -p "$WORK_DIR/logs"
  cat >"$PROFILE" <<EOF
proxies:
  - name: "demo-proxy"
    type: http
    server: "127.0.0.1"
    port: 18080
  - name: "egress-proxy"
    type: http
    server: "127.0.0.1"
    port: $EGRESS_PROXY_PORT

proxy-providers:
  demo-provider:
    type: file
    path: ./provider.yaml
    health-check:
      enable: false
  remote-provider:
    type: http
    url: http://127.0.0.1:$EGRESS_ORIGIN_PORT/remote-provider.yaml
    path: ./remote-provider-cache.yaml
    interval: 3600
    health-check:
      enable: false

proxy-groups:
  - name: "Proxy"
    type: select
    use:
      - demo-provider
      - remote-provider
    proxies:
      - "demo-proxy"
      - DIRECT
  - name: "EgressSwitch"
    type: select
    proxies:
      - DIRECT
      - "egress-proxy"

rules:
  - IP-CIDR,127.0.0.1/32,EgressSwitch,no-resolve
  - DOMAIN,example.com,Proxy
  - MATCH,DIRECT
EOF

  cat >"$PROVIDER" <<'EOF'
proxies:
  - name: "provider-proxy"
    type: http
    server: "127.0.0.1"
    port: 18081
EOF

  cat >"$REMOTE_PROVIDER" <<'EOF'
proxies:
  - name: "remote-provider-proxy"
    type: http
    server: "127.0.0.1"
    port: 18083
EOF

  cat >"$DEVICE_POLICY" <<'EOF'
{
  "profiles": [
    {
      "id": "integration-egress",
      "default_policies": ["DIRECT", "EgressSwitch"]
    }
  ],
  "devices": [
    {
      "id": "integration-dedicated",
      "mac": "aa:bb:cc:dd:ee:01",
      "ipv4": "192.168.50.101",
      "profile": "integration-egress",
      "egress_mode": "dedicated"
    },
    {
      "id": "integration-inherited",
      "mac": "aa:bb:cc:dd:ee:02",
      "ipv4": "192.168.50.102",
      "profile": "integration-egress",
      "egress_mode": "inherit_global"
    }
  ]
}
EOF

  cat >"$CONFIG" <<EOF
mihomo:
  binary: "$MIHOMO_BINARY"
  config: "$MIHOMO_CONFIG"
  profile_mode: "imported"
  profile: "$PROFILE"
  mixed_port: $MIXED_PORT
  api_addr: "$API_ADDR"
  secret: ""

runtime:
  dir: "$WORK_DIR"

device_policy:
  file: "$DEVICE_POLICY"
EOF
}

build_omg() {
  GOCACHE="${GOCACHE:-$WORK_DIR/go-cache}" go build -o "$OMG_BIN" ./cmd/omg
  GOCACHE="${GOCACHE:-$WORK_DIR/go-cache}" go build -o "$EGRESS_PROBE_BIN" ./tests/integration/egressprobe
}

start_mihomo() {
  local label=$1
  printf '\n# %s\n' "$label" >>"$MIHOMO_LOG"
  "$MIHOMO_BINARY" -d "$WORK_DIR" -f "$MIHOMO_CONFIG" >>"$MIHOMO_LOG" 2>&1 &
  PID=$!
  wait_for_api
}

start_egress_probe() {
  local log_file="$WORK_DIR/logs/egress-probe.log"
  "$EGRESS_PROBE_BIN" \
    --origin "127.0.0.1:$EGRESS_ORIGIN_PORT" \
    --proxy "127.0.0.1:$EGRESS_PROXY_PORT" \
    --provider-file "$REMOTE_PROVIDER" \
    --provider-path "/remote-provider.yaml" \
    --log-dir "$WORK_DIR/egress" >"$log_file" 2>&1 &
  EGRESS_PROBE_PID=$!
  local attempt
  for attempt in $(seq 1 50); do
    if grep -Fq READY "$log_file" 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$EGRESS_PROBE_PID" 2>/dev/null; then
      echo "egress probe exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      return 1
    fi
    sleep 0.1
  done
  echo "egress probe did not become ready" >&2
  cat "$log_file" >&2 || true
  return 1
}

stop_mihomo() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  PID=""
}

wait_for_api() {
  local url="http://$API_ADDR/version"
  local attempt
  for attempt in $(seq 1 50); do
    if curl --fail --silent --show-error --max-time 1 "$url" >/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  echo "mihomo API did not become ready at $url" >&2
  tail -n 120 "$MIHOMO_LOG" 2>/dev/null || true
  return 1
}

wait_for_policy_option() {
  local group=$1
  local option=$2
  local output="$WORK_DIR/policies-wait.json"
  local attempt
  for attempt in $(seq 1 50); do
    if "$OMG_BIN" policies --config "$CONFIG" --format json >"$output" 2>"$WORK_DIR/policies-wait.err" &&
      grep -Fq -- "\"name\": \"$group\"" "$output" &&
      grep -Fq -- "\"$option\"" "$output"; then
      return 0
    fi
    sleep 0.1
  done
  echo "policy group $group did not include option $option" >&2
  cat "$output" >&2 || true
  cat "$WORK_DIR/policies-wait.err" >&2 || true
  tail -n 120 "$MIHOMO_LOG" 2>/dev/null || true
  return 1
}

assert_file_contains() {
  local file=$1
  local pattern=$2
  if ! grep -Fq -- "$pattern" "$file"; then
    echo "missing $pattern in $file" >&2
    cat "$file" >&2
    exit 1
  fi
}

require_file "$MIHOMO_BINARY"

section "write fixture"
write_fixture
printf 'config: %s\n' "${CONFIG#$ROOT/}"

section "build omg"
build_omg
printf 'binary: %s\n' "${OMG_BIN#$ROOT/}"

section "start egress probe"
start_egress_probe
printf 'origin: 127.0.0.1:%s\n' "$EGRESS_ORIGIN_PORT"
printf 'proxy: 127.0.0.1:%s\n' "$EGRESS_PROXY_PORT"

section "validate mihomo config"
"$OMG_BIN" validate-mihomo --config "$CONFIG" --format json
assert_file_contains "$MIHOMO_CONFIG" "store-selected: true"
assert_file_contains "$MIHOMO_CONFIG" "- DIRECT"
assert_file_contains "$MIHOMO_CONFIG" "name: device/integration-dedicated/default"
assert_file_contains "$MIHOMO_CONFIG" "AND,((SRC-IP-CIDR,192.168.50.101/32),(IP-CIDR,192.168.0.0/16)),DIRECT"
assert_file_contains "$MIHOMO_CONFIG" "SRC-IP-CIDR,192.168.50.101/32,device/integration-dedicated/default"
if grep -Fq -- "device/integration-inherited/default" "$MIHOMO_CONFIG"; then
  echo "inherit_global integration fixture unexpectedly generated a default selector" >&2
  exit 1
fi

section "start mihomo"
start_mihomo "initial start"
wait_for_policy_option Proxy remote-provider-proxy

section "list policies"
"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-before.json"
cat "$WORK_DIR/policies-before.json"
assert_file_contains "$WORK_DIR/policies-before.json" '"name": "Proxy"'
assert_file_contains "$WORK_DIR/policies-before.json" '"selected": "demo-proxy"'
assert_file_contains "$WORK_DIR/policies-before.json" '"DIRECT"'
assert_file_contains "$WORK_DIR/policies-before.json" '"remote-provider-proxy"'

section "reject unknown policy"
if "$OMG_BIN" policy-select --config "$CONFIG" --group Proxy --policy Missing --format json >"$WORK_DIR/policy-select-invalid.out" 2>"$WORK_DIR/policy-select-invalid.json"; then
  echo "policy-select accepted an unknown policy" >&2
  cat "$WORK_DIR/policy-select-invalid.out" >&2
  exit 1
fi
cat "$WORK_DIR/policy-select-invalid.json"
assert_file_contains "$WORK_DIR/policy-select-invalid.json" '"command": "policy-select"'
assert_file_contains "$WORK_DIR/policy-select-invalid.json" '"ok": false'
assert_file_contains "$WORK_DIR/policy-select-invalid.json" 'policy \"Missing\" is not a member of group \"Proxy\"'
assert_file_contains "$WORK_DIR/policy-select-invalid.json" 'demo-proxy, DIRECT'

section "select DIRECT"
"$OMG_BIN" policy-select --config "$CONFIG" --group Proxy --policy DIRECT --format json >"$WORK_DIR/policy-select.json"
cat "$WORK_DIR/policy-select.json"
assert_file_contains "$WORK_DIR/policy-select.json" '"selected": "DIRECT"'

section "verify selected policy"
"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-after.json"
cat "$WORK_DIR/policies-after.json"
assert_file_contains "$WORK_DIR/policies-after.json" '"selected": "DIRECT"'

section "restart mihomo"
stop_mihomo
start_mihomo "restart after policy-select"
wait_for_policy_option Proxy remote-provider-proxy

section "verify restored policy"
"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-restored.json"
cat "$WORK_DIR/policies-restored.json"
assert_file_contains "$WORK_DIR/policies-restored.json" '"name": "Proxy"'
assert_file_contains "$WORK_DIR/policies-restored.json" '"selected": "DIRECT"'
assert_file_contains "$WORK_DIR/policies-restored.json" '"DIRECT"'
assert_file_contains "$WORK_DIR/policies-restored.json" '"remote-provider-proxy"'

section "verify policy egress switch"
curl --noproxy '' --proxy "http://127.0.0.1:$MIXED_PORT" --fail --silent --show-error --max-time 5 \
  "http://127.0.0.1:$EGRESS_ORIGIN_PORT/egress-direct" >"$WORK_DIR/egress-direct.out"
cat "$WORK_DIR/egress-direct.out"
assert_file_contains "$WORK_DIR/egress-direct.out" "origin-ok"
assert_file_contains "$WORK_DIR/egress/origin.log" "GET /egress-direct"
if [[ -s "$WORK_DIR/egress/proxy.log" ]]; then
  echo "DIRECT egress unexpectedly used the controlled proxy" >&2
  cat "$WORK_DIR/egress/proxy.log" >&2
  exit 1
fi

"$OMG_BIN" policy-select --config "$CONFIG" --group EgressSwitch --policy egress-proxy --format json >"$WORK_DIR/policy-egress-proxy.json"
cat "$WORK_DIR/policy-egress-proxy.json"
assert_file_contains "$WORK_DIR/policy-egress-proxy.json" '"group": "EgressSwitch"'
assert_file_contains "$WORK_DIR/policy-egress-proxy.json" '"selected": "egress-proxy"'

curl --noproxy '' --proxy "http://127.0.0.1:$MIXED_PORT" --fail --silent --show-error --max-time 5 \
  "http://127.0.0.1:$EGRESS_ORIGIN_PORT/egress-proxy" >"$WORK_DIR/egress-proxy.out"
cat "$WORK_DIR/egress-proxy.out"
assert_file_contains "$WORK_DIR/egress-proxy.out" "origin-ok"
assert_file_contains "$WORK_DIR/egress/origin.log" "GET /egress-proxy"
assert_file_contains "$WORK_DIR/egress/proxy.log" "CONNECT 127.0.0.1:$EGRESS_ORIGIN_PORT"
assert_file_contains "$MIHOMO_LOG" "using EgressSwitch[DIRECT]"
assert_file_contains "$MIHOMO_LOG" "using EgressSwitch[egress-proxy]"

"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-egress.json"
cat "$WORK_DIR/policies-egress.json"
assert_file_contains "$WORK_DIR/policies-egress.json" '"name": "EgressSwitch"'
assert_file_contains "$WORK_DIR/policies-egress.json" '"selected": "egress-proxy"'

"$OMG_BIN" policy-select --config "$CONFIG" --group EgressSwitch --policy DIRECT --format json >"$WORK_DIR/policy-egress-direct.json"
cat "$WORK_DIR/policy-egress-direct.json"
assert_file_contains "$WORK_DIR/policy-egress-direct.json" '"group": "EgressSwitch"'
assert_file_contains "$WORK_DIR/policy-egress-direct.json" '"selected": "DIRECT"'

section "connections"
"$OMG_BIN" connections --config "$CONFIG" --format json >"$WORK_DIR/connections.json"
cat "$WORK_DIR/connections.json"
assert_file_contains "$WORK_DIR/connections.json" '"connections"'

section "providers"
"$OMG_BIN" providers --config "$CONFIG" --format json >"$WORK_DIR/providers.json"
cat "$WORK_DIR/providers.json"
assert_file_contains "$WORK_DIR/providers.json" '"proxy_providers"'
assert_file_contains "$WORK_DIR/providers.json" '"name": "demo-provider"'
assert_file_contains "$WORK_DIR/providers.json" '"vehicle_type": "File"'
assert_file_contains "$WORK_DIR/providers.json" '"proxy_count": 1'
assert_file_contains "$WORK_DIR/providers.json" '"name": "provider-proxy"'
assert_file_contains "$WORK_DIR/providers.json" '"name": "remote-provider"'
assert_file_contains "$WORK_DIR/providers.json" '"name": "remote-provider-proxy"'
assert_file_contains "$WORK_DIR/providers.json" '"rule_providers"'
assert_file_contains "$WORK_DIR/egress/origin.log" "GET /remote-provider.yaml"

section "update remote provider"
cat >"$REMOTE_PROVIDER" <<'EOF'
proxies:
  - name: "remote-provider-updated"
    type: http
    server: "127.0.0.1"
    port: 18084
EOF
"$OMG_BIN" provider-update --config "$CONFIG" --provider remote-provider --format json >"$WORK_DIR/remote-provider-update.json"
cat "$WORK_DIR/remote-provider-update.json"
assert_file_contains "$WORK_DIR/remote-provider-update.json" '"provider": "remote-provider"'
assert_file_contains "$WORK_DIR/remote-provider-update.json" '"updated": true'
assert_file_contains "$WORK_DIR/remote-provider-update.json" '"name": "remote-provider-updated"'

"$OMG_BIN" providers --config "$CONFIG" --format json >"$WORK_DIR/remote-providers-updated.json"
cat "$WORK_DIR/remote-providers-updated.json"
assert_file_contains "$WORK_DIR/remote-providers-updated.json" '"name": "remote-provider"'
assert_file_contains "$WORK_DIR/remote-providers-updated.json" '"name": "remote-provider-updated"'

"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-remote-provider-updated.json"
cat "$WORK_DIR/policies-remote-provider-updated.json"
assert_file_contains "$WORK_DIR/policies-remote-provider-updated.json" '"selected": "DIRECT"'
assert_file_contains "$WORK_DIR/policies-remote-provider-updated.json" '"remote-provider-updated"'

section "update file provider"
cat >"$PROVIDER" <<'EOF'
proxies:
  - name: "provider-updated"
    type: http
    server: "127.0.0.1"
    port: 18082
EOF
"$OMG_BIN" provider-update --config "$CONFIG" --provider demo-provider --format json >"$WORK_DIR/provider-update.json"
cat "$WORK_DIR/provider-update.json"
assert_file_contains "$WORK_DIR/provider-update.json" '"provider": "demo-provider"'
assert_file_contains "$WORK_DIR/provider-update.json" '"updated": true'
assert_file_contains "$WORK_DIR/provider-update.json" '"name": "provider-updated"'

"$OMG_BIN" providers --config "$CONFIG" --format json >"$WORK_DIR/providers-updated.json"
cat "$WORK_DIR/providers-updated.json"
assert_file_contains "$WORK_DIR/providers-updated.json" '"name": "demo-provider"'
assert_file_contains "$WORK_DIR/providers-updated.json" '"name": "provider-updated"'

"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-provider-updated.json"
cat "$WORK_DIR/policies-provider-updated.json"
assert_file_contains "$WORK_DIR/policies-provider-updated.json" '"selected": "DIRECT"'
assert_file_contains "$WORK_DIR/policies-provider-updated.json" '"provider-updated"'
assert_file_contains "$WORK_DIR/policies-provider-updated.json" '"remote-provider-updated"'

section "snapshot"
"$OMG_BIN" snapshot --config "$CONFIG" --tail 5 --format json >"$WORK_DIR/snapshot.json"
cat "$WORK_DIR/snapshot.json"
assert_file_contains "$WORK_DIR/snapshot.json" '"status"'
assert_file_contains "$WORK_DIR/snapshot.json" '"doctor"'
assert_file_contains "$WORK_DIR/snapshot.json" '"leases"'
assert_file_contains "$WORK_DIR/snapshot.json" '"logs"'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "mihomo"'
assert_file_contains "$WORK_DIR/snapshot.json" '"exists": true'
assert_file_contains "$WORK_DIR/snapshot.json" '"mihomo"'
assert_file_contains "$WORK_DIR/snapshot.json" '"policies"'
assert_file_contains "$WORK_DIR/snapshot.json" '"available": true'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "Proxy"'
assert_file_contains "$WORK_DIR/snapshot.json" '"connections"'
assert_file_contains "$WORK_DIR/snapshot.json" '"providers"'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "demo-provider"'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "provider-updated"'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "remote-provider"'
assert_file_contains "$WORK_DIR/snapshot.json" '"name": "remote-provider-updated"'

section "done"
printf 'policy-control integration passed\n'
