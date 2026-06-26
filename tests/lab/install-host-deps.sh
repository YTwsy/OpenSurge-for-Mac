#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROXY_ENV="$ROOT/runtime/lab/proxy.env"
TOOLS_ROOT="$ROOT/runtime/tools"
CACHE_ROOT="$TOOLS_ROOT/cache"
LIMA_VERSION=2.1.3
LIMA_SHA256=52bcf0780fcb28128ac9f6924d4410a6bc7c92fa80c9a858d89ae34ec3ce4f35
LIMA_SIZE=37442481
LIMA_ARCHIVE="lima-${LIMA_VERSION}-Darwin-arm64.tar.gz"
LIMA_URL="https://github.com/lima-vm/lima/releases/download/v${LIMA_VERSION}/${LIMA_ARCHIVE}"
SOCKET_VMNET_VERSION=1.2.2
SOCKET_VMNET_SHA256=c7bf62308fbcfdc29bdfb8373c9b1951f7ac2396446e4390919796a94972e6dc
SOCKET_VMNET_ARCHIVE="socket_vmnet-${SOCKET_VMNET_VERSION}-arm64.tar.gz"
SOCKET_VMNET_URL="https://github.com/lima-vm/socket_vmnet/releases/download/v${SOCKET_VMNET_VERSION}/${SOCKET_VMNET_ARCHIVE}"
DNSMASQ_VERSION=2.93
DNSMASQ_SHA256=cc967771abdafeb43d10db18932d6b59fd4bed2c69c22acf8cb96aff6920d55f
DNSMASQ_ARCHIVE="dnsmasq-${DNSMASQ_VERSION}.tar.gz"
DNSMASQ_URL="https://thekelleys.org.uk/dnsmasq/${DNSMASQ_ARCHIVE}"
MIHOMO_VERSION=1.19.27
MIHOMO_SHA256=3617c9d8a5a55aecfe1ebd0f55ff59f2706c8ad68fd65c6c4e5f7cf2b74263f1
MIHOMO_SIZE=15719003
MIHOMO_ARCHIVE="mihomo-darwin-arm64-v${MIHOMO_VERSION}.gz"
MIHOMO_URL="https://github.com/MetaCubeX/mihomo/releases/download/v${MIHOMO_VERSION}/${MIHOMO_ARCHIVE}"
ALPINE_IMAGE="nocloud_alpine-3.23.4-aarch64-uefi-cloudinit-r0.qcow2"
ALPINE_IMAGE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/cloud/${ALPINE_IMAGE}"
ALPINE_IMAGE_SIZE=235077632
ALPINE_IMAGE_SHA512=737b0eb365944ffcf6da477c059c105bbde222942fd5a8380c07db088365b18dbc60a0f2824ea9a3904d57e194e0d05a7074302c3ebc104d0aecb6ddc6ffd91c
INSTALL_ROOT=/opt/open-mihomo-gateway
NETWORK_HELPER="$INSTALL_ROOT/bin/omg-lab-network"
SUDOERS_FILE=/private/etc/sudoers.d/open-mihomo-gateway-lab
MODE="${1:---all}"
WITH_SUDOERS=0
if [[ "${2:-}" == "--with-sudoers" ]]; then
  WITH_SUDOERS=1
elif [[ $# -gt 1 ]]; then
  echo "usage: $0 [--all|--user-only|--root-only|--uninstall-root] [--with-sudoers]" >&2
  exit 2
fi

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
  echo "the lab installer currently supports Apple Silicon macOS only" >&2
  exit 1
fi
if [[ -f "$PROXY_ENV" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$PROXY_ENV"
  set +a
fi

download_and_verify() {
  local url=$1 output=$2 checksum=$3
  if [[ -f "$output" ]] && printf '%s  %s\n' "$checksum" "$output" | shasum -a 256 --check --status; then
    echo "Using cached: $(basename "$output")"
    return 0
  fi
  curl --fail --location --silent --show-error "$url" -o "$output"
  printf '%s  %s\n' "$checksum" "$output" | shasum -a 256 --check
}

download_range() {
  local mode=$1 url=$2 start=$3 end=$4 output=$5
  if [[ "$mode" == "direct" ]]; then
    env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy \
      -u ALL_PROXY -u all_proxy -u FTP_PROXY -u ftp_proxy -u grpc_proxy \
      curl --fail --location --silent --show-error --retry 3 \
        --connect-timeout 15 --max-time 1200 --range "$start-$end" \
        "$url" -o "$output"
  else
    NO_PROXY='127.0.0.1,localhost,::1' no_proxy='127.0.0.1,localhost,::1' \
      curl --fail --location --silent --show-error --retry 3 \
        --connect-timeout 15 --max-time 1200 --range "$start-$end" \
        "$url" -o "$output"
  fi
}

download_range_parallel() {
  local mode=$1 url=$2 start=$3 end=$4 output=$5 parts=${6:-4}
  local size chunk part_start part_end part_dir i failed pid
  local pids=()
  size=$((end - start + 1))
  chunk=$(((size + parts - 1) / parts))
  part_dir="${output}.chunks"
  rm -rf "$part_dir"
  mkdir -p "$part_dir"
  for ((i = 0; i < parts; i++)); do
    part_start=$((start + i * chunk))
    if ((part_start > end)); then
      break
    fi
    part_end=$((part_start + chunk - 1))
    if ((part_end > end)); then
      part_end=$end
    fi
    download_range "$mode" "$url" "$part_start" "$part_end" "$part_dir/part.$i" &
    pids+=("$!")
  done
  failed=0
  for pid in "${pids[@]}"; do
    wait "$pid" || failed=1
  done
  if ((failed != 0)); then
    return 1
  fi
  : >"$output"
  for ((i = 0; i < ${#pids[@]}; i++)); do
    cat "$part_dir/part.$i" >>"$output"
  done
  rm -rf "$part_dir"
}

download_segmented_and_verify() {
  local url=$1 output=$2 checksum=$3 size=$4 parts=${5:-8} mode=${6:-proxy} algorithm=${7:-256}
  local chunk part_dir start end expected have resume_have resume_start resume_file i failed pid
  local pids=()
  if [[ -f "$output" ]] && printf '%s  %s\n' "$checksum" "$output" | shasum -a "$algorithm" --check --status; then
    echo "Using cached: $(basename "$output")"
    return 0
  fi

  part_dir="${output}.parts-${parts}"
  mkdir -p "$part_dir"
  chunk=$(((size + parts - 1) / parts))
  echo "Downloading $(basename "$output") in $parts verified segments"
  for ((i = 0; i < parts; i++)); do
    start=$((i * chunk))
    end=$((start + chunk - 1))
    if ((end >= size)); then
      end=$((size - 1))
    fi
    expected=$((end - start + 1))
    have=0
    if [[ -f "$part_dir/part.$i" ]]; then
      have=$(stat -f %z "$part_dir/part.$i")
      if ((have > expected)); then
        rm -f "$part_dir/part.$i"
        have=0
      fi
    fi
    resume_file="$part_dir/part.$i.resume"
    if [[ -f "$resume_file" ]]; then
      resume_have=$(stat -f %z "$resume_file")
      if ((have + resume_have <= expected)); then
        cat "$resume_file" >>"$part_dir/part.$i"
        have=$((have + resume_have))
      fi
      rm -f "$resume_file"
    fi
    if ((have == expected)); then
      continue
    fi
    resume_start=$((start + have))
    (
      if ((have > 0)); then
        download_range_parallel "$mode" "$url" "$resume_start" "$end" "$resume_file" 4
      else
        download_range "$mode" "$url" "$resume_start" "$end" "$resume_file"
      fi
      cat "$resume_file" >>"$part_dir/part.$i"
      rm -f "$resume_file"
    ) &
    pids+=("$!")
  done
  failed=0
  for pid in "${pids[@]}"; do
    wait "$pid" || failed=1
  done
  if ((failed != 0)); then
    echo "one or more download segments failed" >&2
    return 1
  fi
  for ((i = 0; i < parts; i++)); do
    start=$((i * chunk))
    end=$((start + chunk - 1))
    if ((end >= size)); then
      end=$((size - 1))
    fi
    expected=$((end - start + 1))
    have=$(stat -f %z "$part_dir/part.$i")
    if ((have != expected)); then
      echo "download segment $i has $have bytes; expected $expected" >&2
      return 1
    fi
  done
  : >"$output"
  for ((i = 0; i < parts; i++)); do
    cat "$part_dir/part.$i" >>"$output"
  done
  printf '%s  %s\n' "$checksum" "$output" | shasum -a "$algorithm" --check
  rm -rf "$part_dir"
}

install_user_tools() {
  local tmpdir socket_vmnet_bin socket_vmnet_client_bin
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' RETURN
  mkdir -p "$TOOLS_ROOT/lima" "$TOOLS_ROOT/bin" "$TOOLS_ROOT/socket_vmnet/bin" "$CACHE_ROOT"

  download_segmented_and_verify "$LIMA_URL" "$CACHE_ROOT/$LIMA_ARCHIVE" "$LIMA_SHA256" "$LIMA_SIZE" 8 direct
  rm -rf "$TOOLS_ROOT/lima/bin" "$TOOLS_ROOT/lima/share"
  tar -xzf "$CACHE_ROOT/$LIMA_ARCHIVE" -C "$TOOLS_ROOT/lima"

  download_and_verify "$DNSMASQ_URL" "$CACHE_ROOT/$DNSMASQ_ARCHIVE" "$DNSMASQ_SHA256"
  tar -xzf "$CACHE_ROOT/$DNSMASQ_ARCHIVE" -C "$tmpdir"
  make -C "$tmpdir/dnsmasq-$DNSMASQ_VERSION" -j"$(sysctl -n hw.logicalcpu)"
  install -m 0755 "$tmpdir/dnsmasq-$DNSMASQ_VERSION/src/dnsmasq" "$TOOLS_ROOT/bin/dnsmasq"

  download_segmented_and_verify "$MIHOMO_URL" "$CACHE_ROOT/$MIHOMO_ARCHIVE" "$MIHOMO_SHA256" "$MIHOMO_SIZE" 8 proxy
  gzip -dc "$CACHE_ROOT/$MIHOMO_ARCHIVE" >"$TOOLS_ROOT/bin/mihomo"
  chmod 0755 "$TOOLS_ROOT/bin/mihomo"

  download_and_verify "$SOCKET_VMNET_URL" "$CACHE_ROOT/$SOCKET_VMNET_ARCHIVE" "$SOCKET_VMNET_SHA256"
  mkdir -p "$tmpdir/socket-vmnet"
  tar -xzf "$CACHE_ROOT/$SOCKET_VMNET_ARCHIVE" -C "$tmpdir/socket-vmnet"
  socket_vmnet_bin="$(find "$tmpdir/socket-vmnet" -type f -name socket_vmnet -perm -111 | head -1)"
  socket_vmnet_client_bin="$(find "$tmpdir/socket-vmnet" -type f -name socket_vmnet_client -perm -111 | head -1)"
  if [[ -z "$socket_vmnet_bin" || -z "$socket_vmnet_client_bin" ]]; then
    echo "socket_vmnet archive did not contain the expected binaries" >&2
    exit 1
  fi
  install -m 0755 "$socket_vmnet_bin" "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet"
  install -m 0755 "$socket_vmnet_client_bin" "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet_client"

  download_segmented_and_verify "$ALPINE_IMAGE_URL" "$CACHE_ROOT/$ALPINE_IMAGE" \
    "$ALPINE_IMAGE_SHA512" "$ALPINE_IMAGE_SIZE" 12 direct 512
  rm -rf "$tmpdir"
  trap - RETURN

  export PATH="$TOOLS_ROOT/lima/bin:$TOOLS_ROOT/bin:$PATH"
  echo "Prepared: $(limactl --version | head -1)"
  echo "Prepared: $(dnsmasq --version | head -1)"
  echo "Prepared: $(mihomo -v | head -1)"
  echo "Prepared: $("$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet" --version)"
}

as_root() {
  if [[ "$EUID" == 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

install_root_components() {
  local install_user sudoers_tmp tmpdir
  install_user="${OMG_LAB_INSTALL_USER:-$(id -un)}"
  if [[ ! "$install_user" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "unsupported username for sudoers rule: $install_user" >&2
    exit 1
  fi
  if [[ ! -x "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet" || ! -x "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet_client" ]]; then
    echo "socket_vmnet binaries are not prepared; run $0 --user-only first" >&2
    exit 1
  fi

  echo "Installing root-owned network components. sudo may prompt once."
  as_root install -d -o root -g wheel -m 0755 /opt/socket_vmnet/bin
  as_root install -o root -g wheel -m 0755 "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet" /opt/socket_vmnet/bin/socket_vmnet
  as_root install -o root -g wheel -m 0755 "$TOOLS_ROOT/socket_vmnet/bin/socket_vmnet_client" /opt/socket_vmnet/bin/socket_vmnet_client
  as_root install -d -o root -g wheel -m 0755 "$INSTALL_ROOT/bin"
  as_root install -o root -g wheel -m 0755 "$ROOT/tests/lab/host/omg-lab-network" "$NETWORK_HELPER"
  as_root touch /private/var/log/open-mihomo-gateway-lab-vmnet.log
  as_root chown root:wheel /private/var/log/open-mihomo-gateway-lab-vmnet.log
  as_root chmod 0644 /private/var/log/open-mihomo-gateway-lab-vmnet.log

  if [[ "$WITH_SUDOERS" == 1 ]]; then
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' RETURN
    sudoers_tmp="$tmpdir/open-mihomo-gateway-lab.sudoers"
    cat >"$sudoers_tmp" <<EOF
$install_user ALL=(root) NOPASSWD: $NETWORK_HELPER start
$install_user ALL=(root) NOPASSWD: $NETWORK_HELPER stop
$install_user ALL=(root) NOPASSWD: $NETWORK_HELPER status
EOF
    as_root visudo -cf "$sudoers_tmp"
    as_root install -o root -g wheel -m 0440 "$sudoers_tmp" "$SUDOERS_FILE"
    rm -rf "$tmpdir"
    trap - RETURN
    echo "Installed restricted sudoers rule for user: $install_user"
  else
    as_root rm -f "$SUDOERS_FILE"
    echo "Skipped sudoers rule; run sudo -v before lab commands that need root."
  fi

  echo "Installed: $(/opt/socket_vmnet/bin/socket_vmnet --version)"
  echo "Installed network helper for user: $install_user"
}

uninstall_root_components() {
  echo "Removing root-owned lab network components. sudo may prompt once."
  if [[ -x "$NETWORK_HELPER" ]]; then
    as_root "$NETWORK_HELPER" stop || true
  fi
  as_root rm -f "$SUDOERS_FILE"
  as_root rm -f /private/var/log/open-mihomo-gateway-lab-vmnet.log
  as_root rm -rf "$INSTALL_ROOT"
  as_root rm -rf /opt/socket_vmnet
  echo "Removed root-owned lab network components"
}

case "$MODE" in
  --all)
    install_user_tools
    install_root_components
    ;;
  --user-only)
    install_user_tools
    ;;
  --root-only)
    install_root_components
    ;;
  --uninstall-root)
    uninstall_root_components
    ;;
  *)
    echo "usage: $0 [--all|--user-only|--root-only|--uninstall-root]" >&2
    exit 2
    ;;
esac

echo "Run: make lab-up"
