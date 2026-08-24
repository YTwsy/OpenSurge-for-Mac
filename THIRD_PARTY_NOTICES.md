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
  `bf3a188a83475000df235178edf61cd70fda22b884b19a539d0cfd9b89a51e6a`
- Upstream: <https://github.com/MetaCubeX/mihomo>
- Corresponding source:
  <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
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

## React, React DOM, and scheduler

- Versions: React `19.2.7`, React DOM `19.2.7`, scheduler `0.27.0`
- License: MIT
- Upstream: <https://github.com/facebook/react>
- License text: [`third_party/licenses/react-MIT.txt`](third_party/licenses/react-MIT.txt)
