# Third-Party Notices

OpenSurge for Mac is licensed under `GPL-3.0-only`. The independent programs
and libraries listed below retain their upstream licenses. The macOS installer
places this notice and the referenced license texts under
`/Library/Application Support/OpenSurge/share/licenses/`.

## mihomo

- Upstream base version: `1.19.30`
- Distributed OpenSurge build version: `1.19.30-opensurge.1`
- License: `GPL-3.0-only`
- Distributed form: architecture-specific binaries compiled from the pinned
  upstream source with the OpenSurge packet-listener patch stored under
  `patches/mihomo`. The listener reuses mihomo's existing gVisor/sing-tun data
  plane for the OpenSurge downstream IPv6 packet broker.
- Source archive SHA-256:
  `971dd4533e4e2c3dad7473e8115200da8c0d7471b4b61da54da896345c5b3850`
- Upstream: <https://github.com/MetaCubeX/mihomo>
- Corresponding source:
  <https://github.com/MetaCubeX/mihomo/tree/ac017cdd246ce8bd547653d927e7bf77d7ee73d5>
- License text: [`LICENSE`](LICENSE)

## dnsmasq

- Version: `2.93`
- License: `GPL-2.0-only OR GPL-3.0-only`, at the recipient's option
- Distributed form: built from unmodified upstream source for Apple Silicon or
  Intel macOS by [`scripts/prepare-gui-release-deps.sh`](scripts/prepare-gui-release-deps.sh)
- Source archive SHA-256:
  `cc967771abdafeb43d10db18932d6b59fd4bed2c69c22acf8cb96aff6920d55f`
- Corresponding source:
  <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
- License texts: [`third_party/licenses/dnsmasq-COPYING`](third_party/licenses/dnsmasq-COPYING)
  and [`LICENSE`](LICENSE)

## gopkg.in/yaml.v3

- Version: `3.0.1`
- License: MIT and Apache-2.0, according to the upstream per-file notice
- Upstream: <https://github.com/go-yaml/yaml/tree/v3.0.1>
- License texts: [`third_party/licenses/yaml-v3-LICENSE`](third_party/licenses/yaml-v3-LICENSE)
  and [`third_party/licenses/Apache-2.0.txt`](third_party/licenses/Apache-2.0.txt)

## github.com/metacubex/bbolt

- Version: `v0.0.0-20260706163408-d4ec34ad7c48`, matching the pinned Mihomo cache implementation
- Use: read-only recovery of device selector choices from the core's cache
- License: MIT
- Upstream: <https://github.com/metacubex/bbolt/tree/d4ec34ad7c48>
- License text: [`third_party/licenses/bbolt-MIT.txt`](third_party/licenses/bbolt-MIT.txt)

## React, React DOM, and scheduler

- Versions: React `19.2.7`, React DOM `19.2.7`, scheduler `0.27.0`
- License: MIT
- Upstream: <https://github.com/facebook/react>
- License text: [`third_party/licenses/react-MIT.txt`](third_party/licenses/react-MIT.txt)

## Wails desktop host

- Version: `v3.0.0-beta.26`, pinned by the independent `apps/desktop/go.mod`
- License: MIT
- Use: native window, tray and system WebView hosting; no Chromium/Node runtime
- Upstream: <https://github.com/wailsapp/wails/tree/v3.0.0-beta.26>
- License text: [`third_party/licenses/wails-MIT.txt`](third_party/licenses/wails-MIT.txt)
