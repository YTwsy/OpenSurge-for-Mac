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
MIHOMO_SHA256=3617c9d8a5a55aecfe1ebd0f55ff59f2706c8ad68fd65c6c4e5f7cf2b74263f1
MIHOMO_ARCHIVE="mihomo-darwin-arm64-v${MIHOMO_VERSION}.gz"
MIHOMO_URL="https://github.com/MetaCubeX/mihomo/releases/download/v${MIHOMO_VERSION}/${MIHOMO_ARCHIVE}"

if [[ "$(uname -s)" != "Darwin" || "$RELEASE_ARCH" != "arm64" ]]; then
  echo "unsigned GUI release dependencies currently support Apple Silicon macOS only" >&2
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
dnsmasq_make_args=()
if [[ "$(uname -m)" != "$RELEASE_ARCH" ]]; then
  dnsmasq_make_args+=("CC=clang -arch $RELEASE_ARCH")
fi
MACOSX_DEPLOYMENT_TARGET="$MINIMUM_MACOS" \
  make -C "$work_dir/dnsmasq-$DNSMASQ_VERSION" -j"$build_jobs" "${dnsmasq_make_args[@]}"
install -m 0755 "$work_dir/dnsmasq-$DNSMASQ_VERSION/src/dnsmasq" "$BIN_ROOT/dnsmasq"

download_and_verify "$MIHOMO_URL" "$CACHE_ROOT/$MIHOMO_ARCHIVE" "$MIHOMO_SHA256"
gzip -dc "$CACHE_ROOT/$MIHOMO_ARCHIVE" >"$BIN_ROOT/mihomo"
chmod 0755 "$BIN_ROOT/mihomo"

for executable in "$BIN_ROOT/dnsmasq" "$BIN_ROOT/mihomo"; do
  /usr/bin/lipo "$executable" -verify_arch "$RELEASE_ARCH"
done

echo "Prepared: $("$BIN_ROOT/dnsmasq" --version | head -1)"
echo "Prepared: $("$BIN_ROOT/mihomo" -v | head -1)"
echo "Release dependency directory: $BIN_ROOT"
