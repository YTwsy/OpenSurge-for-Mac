#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_ROOT="${OPENSURGE_RELEASE_TOOLS_ROOT:-$ROOT/runtime/release-tools}"
CACHE_ROOT="$OUTPUT_ROOT/cache"
BIN_ROOT="$OUTPUT_ROOT/bin"
RELEASE_ARCH="${OPENSURGE_RELEASE_ARCH:-$(uname -m)}"
MINIMUM_MACOS="${OPENSURGE_MINIMUM_MACOS:-13.0}"

DNSMASQ_VERSION=2.93
DNSMASQ_SHA256=cc967771abdafeb43d10db18932d6b59fd4bed2c69c22acf8cb96aff6920d55f
DNSMASQ_ARCHIVE="dnsmasq-${DNSMASQ_VERSION}.tar.gz"
DNSMASQ_URL="https://thekelleys.org.uk/dnsmasq/${DNSMASQ_ARCHIVE}"

MIHOMO_VERSION=1.19.27
MIHOMO_SOURCE_SHA256=bf3a188a83475000df235178edf61cd70fda22b884b19a539d0cfd9b89a51e6a
MIHOMO_SOURCE_ARCHIVE="mihomo-${MIHOMO_VERSION}-source.tar.gz"
MIHOMO_SOURCE_URL="https://github.com/MetaCubeX/mihomo/archive/5184081ac327394d9e15fa5d5f9f4a61e723fd94.tar.gz"
case "$RELEASE_ARCH" in
  arm64|x86_64)
    ;;
  *)
    echo "unsupported macOS release architecture: $RELEASE_ARCH" >&2
    exit 1
    ;;
esac

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "GUI release dependencies must be prepared on macOS" >&2
  exit 1
fi

mkdir -p "$CACHE_ROOT" "$BIN_ROOT"
work_dir="$(mktemp -d "${TMPDIR:-/private/tmp}/opensurge-release-deps.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

download_and_verify() {
  local url=$1 output=$2 checksum=$3
  if [[ ! -f "$output" ]] || ! printf '%s  %s\n' "$checksum" "$output" | shasum -a 256 --check --status; then
    curl --fail --location --silent --show-error --retry 3 \
      --connect-timeout 15 --max-time 1200 "$url" -o "$output"
  fi
  printf '%s  %s\n' "$checksum" "$output" | shasum -a 256 --check
}

download_and_verify "$DNSMASQ_URL" "$CACHE_ROOT/$DNSMASQ_ARCHIVE" "$DNSMASQ_SHA256"
tar -xzf "$CACHE_ROOT/$DNSMASQ_ARCHIVE" -C "$work_dir"
build_jobs="$(sysctl -n hw.logicalcpu 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 1)"
MACOSX_DEPLOYMENT_TARGET="$MINIMUM_MACOS" \
  make -C "$work_dir/dnsmasq-$DNSMASQ_VERSION" -j"$build_jobs" \
    "CC=clang -arch $RELEASE_ARCH"
install -m 0755 "$work_dir/dnsmasq-$DNSMASQ_VERSION/src/dnsmasq" "$BIN_ROOT/dnsmasq"

MIHOMO_SOURCE="$CACHE_ROOT/$MIHOMO_SOURCE_ARCHIVE"
download_and_verify "$MIHOMO_SOURCE_URL" "$MIHOMO_SOURCE" "$MIHOMO_SOURCE_SHA256"
OPENSURGE_MIHOMO_SOURCE_ARCHIVE="$MIHOMO_SOURCE" \
OPENSURGE_MIHOMO_OUTPUT="$BIN_ROOT/mihomo" \
OPENSURGE_MIHOMO_ARCH="$RELEASE_ARCH" \
  "$ROOT/scripts/build-opensurge-mihomo.sh"

for executable in "$BIN_ROOT/dnsmasq" "$BIN_ROOT/mihomo"; do
  if [[ ! -x "$executable" ]]; then
    echo "release dependency was not prepared: $executable" >&2
    exit 1
  fi
  /usr/bin/lipo "$executable" -verify_arch "$RELEASE_ARCH"
done

if [[ "$(uname -m)" == "$RELEASE_ARCH" ]]; then
  echo "Prepared: $("$BIN_ROOT/dnsmasq" --version | head -1)"
  echo "Prepared: $("$BIN_ROOT/mihomo" -v | head -1)"
else
  echo "Prepared dnsmasq $DNSMASQ_VERSION for $RELEASE_ARCH (cross-compiled)"
  echo "Prepared OpenSurge mihomo $MIHOMO_VERSION for $RELEASE_ARCH from pinned source"
fi
echo "Release dependency directory: $BIN_ROOT"
