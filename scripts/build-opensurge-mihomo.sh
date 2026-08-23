#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_ARCHIVE_OVERRIDE="${OPENSURGE_MIHOMO_SOURCE_ARCHIVE:-}"
SOURCE_ARCHIVE="${SOURCE_ARCHIVE_OVERRIDE:-$ROOT/runtime/release-tools/cache/mihomo-1.19.30-source.tar.gz}"
SOURCE_SHA256=971dd4533e4e2c3dad7473e8115200da8c0d7471b4b61da54da896345c5b3850
SOURCE_URL=https://github.com/MetaCubeX/mihomo/archive/ac017cdd246ce8bd547653d927e7bf77d7ee73d5.tar.gz
OUTPUT="${OPENSURGE_MIHOMO_OUTPUT:-$ROOT/bin/mihomo}"
TARGET_ARCH="${OPENSURGE_MIHOMO_ARCH:-$(uname -m)}"
VERSION="${OPENSURGE_MIHOMO_VERSION:-1.19.30-opensurge.1}"
GO_BIN="${GO_BIN:-$(command -v go || true)}"

[[ -x "$GO_BIN" ]] || { echo "Go toolchain not found" >&2; exit 1; }
case "$TARGET_ARCH" in
  arm64) GO_ARCH=arm64 ;;
  x86_64|amd64) GO_ARCH=amd64 ;;
  *) echo "unsupported mihomo architecture: $TARGET_ARCH" >&2; exit 1 ;;
esac

source_archive_valid() {
  [[ -f "$SOURCE_ARCHIVE" ]] && printf '%s  %s\n' "$SOURCE_SHA256" "$SOURCE_ARCHIVE" | shasum -a 256 --check --status
}

if ! source_archive_valid; then
  if [[ -n "$SOURCE_ARCHIVE_OVERRIDE" ]]; then
    echo "explicit mihomo source archive is missing or does not match the pinned SHA-256: $SOURCE_ARCHIVE" >&2
    exit 1
  fi
  command -v curl >/dev/null 2>&1 || { echo "curl is required to download the pinned mihomo source" >&2; exit 1; }
  mkdir -p "$(dirname "$SOURCE_ARCHIVE")"
  curl --fail --location --silent --show-error --retry 3 \
    --connect-timeout 15 --max-time 1200 "$SOURCE_URL" -o "$SOURCE_ARCHIVE"
  source_archive_valid || { echo "downloaded mihomo source does not match the pinned SHA-256" >&2; exit 1; }
fi

work_dir="$(mktemp -d "${TMPDIR:-/private/tmp}/opensurge-mihomo.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
tar -xzf "$SOURCE_ARCHIVE" -C "$work_dir" --strip-components=1
patch -d "$work_dir" -p1 <"$ROOT/patches/mihomo/0001-opensurge-packet-listener.patch"
cp -R "$ROOT/patches/mihomo/overlay/listener" "$work_dir/"
mkdir -p "$(dirname "$OUTPUT")"

build_output="$work_dir/mihomo-opensurge"

cd "$work_dir"
GOOS=darwin GOARCH="$GO_ARCH" CGO_ENABLED=0 \
  "$GO_BIN" build -tags with_gvisor -trimpath \
  -ldflags "-X github.com/metacubex/mihomo/constant.Version=$VERSION -w -s -buildid=" \
  -o "$build_output" .
[[ -x "$build_output" ]] || { echo "Go build completed without producing an executable" >&2; exit 1; }
install -m 0755 "$build_output" "$OUTPUT"

echo "Built OpenSurge mihomo $VERSION for $TARGET_ARCH: $OUTPUT"
