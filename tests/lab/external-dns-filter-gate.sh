#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export OMG_LAB_EXTERNAL_DNS_FILTER=true
if [[ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" == 1 ]]; then
  export OPENSURGE_MIHOMO_ARCH="${OPENSURGE_MIHOMO_ARCH:-arm64}"
fi
sudo -v
cleanup() {
  result=$?
  trap - EXIT INT TERM
  make lab-down || result=1
  make lab-status || result=1
  exit "$result"
}
trap cleanup EXIT INT TERM
make lab-up
for target in lab-test lab-test-tun lab-test-ipv6-userspace lab-test-ipv6-same-wifi lab-test-ipv6-same-lan; do
  sudo -v
  make "$target"
done
