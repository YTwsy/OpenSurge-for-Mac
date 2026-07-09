#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

BASE_DIR="$ROOT/runtime/integration/policy-control"
WORK_DIR="${OMG_POLICY_CONTROL_WORK_DIR:-$BASE_DIR/run-$$}"
CONFIG="$WORK_DIR/config.yaml"
PROFILE="$WORK_DIR/profile.yaml"
MIHOMO_CONFIG="$WORK_DIR/mihomo.yaml"
MIHOMO_LOG="$WORK_DIR/mihomo.log"
OMG_BIN="$WORK_DIR/omg"
MIHOMO_BINARY="${OMG_POLICY_CONTROL_MIHOMO_BINARY:-$ROOT/runtime/tools/bin/mihomo}"
API_ADDR="${OMG_POLICY_CONTROL_API_ADDR:-127.0.0.1:19091}"
MIXED_PORT="${OMG_POLICY_CONTROL_MIXED_PORT:-19092}"
PID=""

section() {
  printf '\n== %s ==\n' "$1"
}

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
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
  mkdir -p "$WORK_DIR"
  cat >"$PROFILE" <<'EOF'
proxies:
  - name: "demo-proxy"
    type: http
    server: "127.0.0.1"
    port: 18080

proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - "demo-proxy"
      - DIRECT

rules:
  - DOMAIN,example.com,Proxy
  - MATCH,DIRECT
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
EOF
}

build_omg() {
  GOCACHE="${GOCACHE:-$WORK_DIR/go-cache}" go build -o "$OMG_BIN" ./cmd/omg
}

start_mihomo() {
  local label=$1
  printf '\n# %s\n' "$label" >>"$MIHOMO_LOG"
  "$MIHOMO_BINARY" -d "$WORK_DIR" -f "$MIHOMO_CONFIG" >>"$MIHOMO_LOG" 2>&1 &
  PID=$!
  wait_for_api
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

section "validate mihomo config"
"$OMG_BIN" validate-mihomo --config "$CONFIG" --format json
assert_file_contains "$MIHOMO_CONFIG" "store-selected: true"
assert_file_contains "$MIHOMO_CONFIG" "- DIRECT"

section "start mihomo"
start_mihomo "initial start"

section "list policies"
"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-before.json"
cat "$WORK_DIR/policies-before.json"
assert_file_contains "$WORK_DIR/policies-before.json" '"name": "Proxy"'
assert_file_contains "$WORK_DIR/policies-before.json" '"selected": "demo-proxy"'
assert_file_contains "$WORK_DIR/policies-before.json" '"DIRECT"'

section "reject unknown policy"
if "$OMG_BIN" policy-select --config "$CONFIG" --group Proxy --policy Missing >"$WORK_DIR/policy-select-invalid.json" 2>"$WORK_DIR/policy-select-invalid.err"; then
  echo "policy-select accepted an unknown policy" >&2
  cat "$WORK_DIR/policy-select-invalid.json" >&2
  exit 1
fi
cat "$WORK_DIR/policy-select-invalid.err"
assert_file_contains "$WORK_DIR/policy-select-invalid.err" 'policy "Missing" is not a member of group "Proxy"'
assert_file_contains "$WORK_DIR/policy-select-invalid.err" 'demo-proxy, DIRECT'

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

section "verify restored policy"
"$OMG_BIN" policies --config "$CONFIG" --format json >"$WORK_DIR/policies-restored.json"
cat "$WORK_DIR/policies-restored.json"
assert_file_contains "$WORK_DIR/policies-restored.json" '"name": "Proxy"'
assert_file_contains "$WORK_DIR/policies-restored.json" '"selected": "DIRECT"'
assert_file_contains "$WORK_DIR/policies-restored.json" '"DIRECT"'

section "connections"
"$OMG_BIN" connections --config "$CONFIG" --format json >"$WORK_DIR/connections.json"
cat "$WORK_DIR/connections.json"
assert_file_contains "$WORK_DIR/connections.json" '"connections"'

section "done"
printf 'policy-control integration passed\n'
