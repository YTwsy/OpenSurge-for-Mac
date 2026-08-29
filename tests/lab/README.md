# Virtual LAN lab

[简体中文](README.zh-CN.md) | English

This lab keeps the gateway under test on macOS and uses two Lima Ubuntu VMs as
independent LAN clients. It does not replace the macOS implementation with a
Linux router.

```text
Internet
   |
macOS upstream interface
   |
real omg + pf + dnsmasq + mihomo
   |
vmnet host network (192.168.48.0/22, no platform DHCP)
   +-- omg-lab-client-1
   +-- omg-lab-client-2
```

Each client has two NICs. Lima's built-in user-mode NIC remains available for
control and provisioning. The second NIC is the test data plane and requests a
lease from the project's dnsmasq instance.

## One-time installation

```sh
make lab-install
```

For non-interactive automation, the same installer can be split safely:

```sh
./tests/lab/install-host-deps.sh --user-only
./tests/lab/install-host-deps.sh --root-only
```

The installer downloads pinned, checksummed upstream releases into
`runtime/tools`, then:

- installs Lima 2.1.3, dnsmasq 2.93, and mihomo 1.19.27 for this project;
- verifies and installs socket_vmnet 1.2.2 under `/opt/socket_vmnet`;
- installs a fixed-function network helper under `/opt/open-mihomo-gateway`;
- does not install a passwordless sudo rule by default.

If an Apple Silicon agent terminal is running under Rosetta, `uname -m` may
report `x86_64` and the architecture guard will reject `make lab-install`.
Confirm `sysctl -n sysctl.proc_translated` is `1` and
`sysctl -n hw.optional.arm64` is `1`, then run the same installer natively:

```sh
/usr/bin/arch -arm64 /bin/bash ./tests/lab/install-host-deps.sh
```

Do not use that workaround on an Intel Mac.

For unattended setup/teardown of the isolated network, explicitly run
`./tests/lab/install-host-deps.sh --root-only --with-sudoers`. The optional rule
allows only the root-owned helper's `start`, `stop`, and `status` commands; it
never executes scripts or binaries from this writable repository. The gateway
binary still requires a cached sudo credential. Run `make lab-uninstall-root`
to remove the root-owned helper, socket_vmnet copy, lab log, and sudoers rule.

Optional proxy variables can be stored in `runtime/lab/proxy.env`. The
installer and lab commands load that file for host-side operations. Lima VM
provisioning does not receive those proxy variables by default; set
`OMG_LAB_VM_PROXY=1` only when the proxy endpoint is reachable from inside the
VMs.

## Daily workflow

```sh
sudo -v && make lab-up
sudo -v && make lab-test
sudo -v && make lab-test-tun
sudo -v && make lab-test-tun-imported-profile
sudo -v && make lab-test-tun-imported-egress
sudo -v && make lab-test-tun-local-routing
sudo -v && make lab-test-tun-device-policy
sudo -v && \
  OMG_LAB_TAILSCALE_PEER_AUTH_KEY_FILE=/private/path/peer.key \
  OMG_LAB_TAILSCALE_OPEN_SURGE_AUTH_KEY_FILE=/private/path/managed.key \
  make lab-test-tailscale
sudo -v && make lab-test-ipv6-userspace
sudo -v && make lab-down
```

`lab-up` starts the DHCP-free host network and the two clients. `lab-test`
builds the current gateway, starts it with the generated lab config, renews both
client leases, checks routing, DNS, ICMP/NAT, direct HTTPS, and explicit HTTPS
through mihomo `mixed-port`, and then verifies cleanup. Artifacts are written
under `artifacts/lab`. Managed mihomo DNS returns fake IPs even while TUN is
disabled, so the direct HTTPS NAT proof resolves a real public A record and
pins it with `curl --resolve`; the separate gateway-DNS assertion still checks
the fake-IP answer intentionally.

The first `lab-up` is intentionally slower because it downloads the pinned,
checksummed Ubuntu image and installs test tools in both clients. `lab-down`
stops the clients but preserves their Lima disks, so use it for normal cleanup;
later `lab-up` runs reuse those clients. Use `make lab-destroy` only to discard
broken state or intentionally rebuild the VMs. On every boot provisioning
restores guest DNS to the Lima control gateway before the gateway under test is
running, and skips `apt-get update` and package installation when all required
tools are already present. Cold rebuilds provision clients sequentially so two
apt jobs cannot contend for upstream bandwidth; unchanged persistent clients
start in parallel so routine `lab-up` does not add two guest boot times.
Changing `tests/lab/lima/client.yaml` makes Lima delete and cold-rebuild the
affected VMs. A VZ cold boot can be silent for about two minutes before falling
back from vsock SSH to the usernet forwarder and reaching `READY`; do not abort
solely because of that quiet interval. Check
`runtime/tools/lima/bin/limactl list` and `~/.lima/<client>/ha.stderr.log` first.

`lab-test-tun` is the TUN transparent proxy gate. It rewrites the lab config
with `transparent.mode: "tun"`, forwards dnsmasq to mihomo DNS, leaves the
clients without explicit proxy settings, and requires the no-proxy HTTPS
request to appear in `mihomo.log`.

`lab-test-tun-imported-profile` runs the TUN gate with an imported profile
fixture. `lab-test-tun-imported-egress` extends that path with a local HTTP
provider and controlled HTTP CONNECT proxy, then switches `TunEgress` from
`DIRECT` to the controlled proxy through `omg policy-select`. It proves
provider-backed policy selection changes the transparent TUN egress path; it
does not prove a real subscription node or remote exit IP. The controlled
proxy binds both upstream DNS and TCP dialing to the physical upstream
interface so its own traffic cannot re-enter the TUN or use a fake IP.

`lab-test-tun-local-routing` uses the same imported egress fixture to prove the
local-Mac Rule/Global/Direct selectors remain isolated from downstream clients:
local Global can use the controlled proxy while the client follows direct
gateway rules, and local Direct can bypass the proxy while the client still
uses it through gateway rules. It also checks the local TUN source identity,
UDP `REJECT` for an HTTP-only global egress, and that generic `policies` hides
the internal groups. The same gate enables fake-AAAA and sends controlled TCP
and QUIC probes to TEST-NET-1 to verify the exact `DEFAULT-TUN` +
`fdfe:dcba:9876::1/128` local IPv6 identity: Rule reaches the imported rule,
Direct logs `DIRECT`, and HTTP-only Global selects its TCP egress while UDP
logs `REJECT`. It rejects broad fake-IP, downstream IPv6, ULA, or
`opensurge-ipv6` identity matching.

`lab-test-tun-device-policy` uses both clients as independently identified LAN
devices. It runs the bridge and DHCP clients on `/22`, assigns fixed
`192.168.50.101` and `192.168.51.102` leases across the third octet, proves one
`dedicated` device takes its selector before global `MATCH`, and proves one
`inherit_global` device follows global `MATCH` without exposing a default
selector. It creates desired drift, applies a change to dedicated mode with a
real `omg reload`, verifies the applied digest is synchronized, then proves the
two selectors choose different egress paths without affecting each other and a
device-specific IP `REJECT` remains enforced. It is the required data-plane
gate for device identity, routing modes, device defaults, and device overrides. It
also verifies the applied bundle/state digest, both exact DHCP identities,
desired-vs-applied drift after a policy-file edit, and a UDP/443 request through
an HTTP-only selected outbound that must log `REJECT` instead of falling through
to `DIRECT`. The fixture also preserves one raw device without a MAC and proves
DHCP mode keeps it in the applied snapshot for later identity completion while
emitting no reservation, mihomo rule, or active selector for it.
The fixture also keeps one MAC-backed registration from another LAN and proves
it remains in desired state while disappearing from compiled/applied runtime
devices, dnsmasq, Mihomo IPv4 rules/selectors, and IPv6 MAC identity.
It also opens one held connection from each device, changes the first device
selector, and calls the real Control API connection-refresh action. The first
device's old connection must disappear, the second device connection must
remain, and a new first-device connection must use the newly selected egress.
The passing artifact retains that applied snapshot, runtime state, generated
dnsmasq/mihomo configuration, and the initial and post-reload device views so
the boundary remains auditable.
Rule/template/provider compilation stays in unit tests.

### Tailscale outbound gate

`lab-test-tailscale` adds a third, persistent Lima VM named
`omg-lab-ts-peer`. It uses only Lima's built-in NAT/control NIC, never the
`omg0` data plane, and runs an independent official `tailscaled` plus a TCP/UDP
fixture bound only to `tailscale0`:

```text
omg-lab-client-1 (allowed) ─┐
                            ├─ omg0 ─ OpenSurge TUN ─ managed tsnet ─┐
omg-lab-client-2 (rejected) ┘                                       │
                                                                     ├─ Tailnet
Mac Tailscale app (discovery and baseline only) ─────────────────────┤
                                                                     │
Lima NAT/control ─ omg-lab-ts-peer's own tailscaled ─────────────────┘
```

The first setup performs two Tailscale registrations because the peer VM and
OpenSurge-managed tsnet are independent nodes. Store auth keys outside the
repository at absolute paths with mode `0600`; do not put a key in a command
argument, an environment-variable value, Git, or an artifact. One-off keys
require two files. A reusable, non-Ephemeral key may instead back both
registrations, with both variables pointing to the same protected file. Run:

```sh
sudo -v && \
  OMG_LAB_TAILSCALE_PEER_AUTH_KEY_FILE=/private/path/peer.key \
  OMG_LAB_TAILSCALE_OPEN_SURGE_AUTH_KEY_FILE=/private/path/managed.key \
  make lab-test-tailscale
```

Both identities persist locally: the peer identity is in its Lima disk and the
managed identity is under `runtime/lab/tailscale`. “One-off” limits key use,
not the number of Lab runs: later runs no longer need the corresponding key.
If a variable remains set, the script only validates the file's safety.
`make lab-tailscale-down` stops the peer while preserving it.
`make lab-tailscale-destroy` deletes the local VM disk and identity. Neither
command removes a device record from the Tailscale admin console; the operator
must remove that record separately. A reusable key mainly makes this destructive
rebuild path automatic until the key expires or is revoked.

The gate verifies that the native Tailscale app is connected without an active
Exit Node; the peer has no `omg0` and does not use Tailscale as its default
route; the Control API discovers the peer and MagicDNS suffix; and, after only
client 1 is authorized, TCP/UDP to the exact peer IP and TCP to the complete
MagicDNS name really use `open-surge/tailscale`. Client 2 must hit `REJECT` for
the same targets. The peer fixture also requires every successful request to
come from one managed Tailnet address that differs from the native Mac app's
address. Before OpenSurge starts, the gate also requires the peer's native
`/32` route to select a `utun`; the distinct observed source then rules out a
false positive through that existing route.

The VM's real Internet underlay is still VM to Lima NAT to the Mac's current
upstream, and it may cross the host TUN after OpenSurge starts. The gate rejects
a native Mac Exit Node to avoid an unnecessary nested exit. An ordinary Mac
underlay does not invalidate the result; the peer-observed Tailnet source and
Mihomo action log jointly identify the application path under test.

This bounded gate proves peer IP, MagicDNS, TCP/UDP, source authorization, and
unauthorized fail-closed behavior only. It does not prove subnet routers, an
Exit Node's public egress, Headscale, or a real remote LAN. Successful runs save
only sanitized rules and boolean evidence; auth keys, full `mihomo.yaml`, raw
logs, Tailnet addresses, and state are excluded from artifacts.
The runtime `mihomo.yaml` that contains the auth key is pre-created as mode
`0600` and checked again after gateway startup. Failed cleanup must not delete
it merely to hide an incomplete gateway rollback.

Use `lab-test-ipv6-userspace` for isolated downstream LAN,
`lab-test-ipv6-same-wifi` for whole-LAN DHCP takeover, and
`lab-test-ipv6-same-lan` for selective bypass-router IPv6. The first two build
the OpenSurge-patched Mihomo and BPF broker, then require both clients to obtain
`fdfe:dcba:9878::/64` SLAAC addresses, a Medium-preference IPv6 default route,
and RDNSS from dnsmasq. The bypass-router gate uses manual ULAs, the Mac
link-local default gateway and DNS, and proves that no RA is configured.
Client one must hit its device-domain `REJECT` over
IPv6 TCP in the isolated-LAN and selective-bypass fixtures. In the
`same_wifi_dhcp` gate it instead uses IPv4 upstream-router bypass and must
report `ipv6_blocked`, emit no selector, and hit the leading packet-listener
`TUN + InUser REJECT`. Client two must hit its own `DIRECT` selector over IPv6 TCP, a
controlled UDP request/response, and a 1200-byte QUIC Initial-shaped UDP
carrier. It must also use an HTTP/3-only client, with no TCP or HTTP/2 fallback,
to complete QUIC TLS and an HTTP/3 request/response through both `DIRECT` and a
controlled SOCKS5 UDP outbound. Selecting the HTTP-only outbound must fail
closed with a UDP `REJECT`, no origin request, and no CONNECT-proxy request.
The TCP and HTTP/3 origins must receive their requests and the UDP fixture must
return its fixed answer; public upstream services are not the sole capture
evidence. Shutdown must withdraw
the OpenSurge default route and remove the Mac gateway alias, broker PID,
Unix sockets, readiness marker, and runtime state. RFC 4862 may temporarily
retain a deprecated/expiring SLAAC address, so immediate address deletion is
not the routing-withdrawal condition. The QUIC-shaped assertion proves only the
UDP carrier and policy match. The HTTP/3-only fixture proves the three bounded
local outbound scenarios above, not every QUIC/HTTP3 version, 0-RTT, connection
migration, public node, or proxy combination.

`lab-test-ipv6-imported-egress` complements that deterministic gate; it does
not replace the local fixtures. Supply an actual mihomo profile explicitly:

```sh
sudo -v && \
  OMG_LAB_IPV6_REAL_PROFILE=/absolute/path/to/profile.yaml \
  make lab-test-ipv6-imported-egress
```

The runner copies the profile to a mode-`0600` file under `runtime/lab`, uses
`tun_ipv6: auto`, and requires status to prove native upstream IPv6 is
available. Client one first hits a MAC/InUser-scoped domain `REJECT`, then in
rule mode completes IPv6-only HTTPS, a public IPv6 UDP DNS response, and a
1200-byte QUIC-shaped UDP `DIRECT` egress. Both the Mac baseline and VM HTTPS
echo must be GUAs currently assigned to the selected Mac upstream interface.
Independent sockets may legitimately select different IPv6 privacy addresses
on that interface, so literal equality is not required. Without printing proxy
names, the runner then selects a real profile leaf that has a distinct exit,
can reach an IPv6 literal, and returns a public IPv6 DNS answer through SOCKS5
UDP ASSOCIATE.
Client two must complete fake-AAAA HTTPS, public IPv6-literal HTTPS, a public
IPv6 UDP DNS response, and a QUIC-shaped UDP `GLOBAL` egress.

The VM address remains an OpenSurge ULA rather than an ISP-delegated public
prefix. `DIRECT` here means that Mihomo/gVisor originates a native IPv6 socket
from the Mac; it is not native forwarding of the VM ULA. The Mac's native IPv6
upstream and GUA, subscription node, and public IPv6 targets are the real
components.

Secret-safe artifacts deliberately omit the copied profile, generated
`mihomo.yaml`, selector/API output, raw Mihomo log, and cache. The gate scans
the retained artifact files for profile server, credential, and sufficiently
long proxy-name markers. Successful cleanup removes the runtime profile,
generated config, log, and selection cache; an incomplete gateway rollback
retains the mode-`0600` runtime material for recovery instead of hiding the
failure.

Treat `make lab-test` as the required local gate for high-risk network changes:
DHCP/DNS behavior, mihomo process or config generation, pf/NAT rules,
forwarding and rollback, gateway lifecycle cleanup, lab topology, or runtime
traffic defaults. The normal CI workflow intentionally stops at `make test`;
run this lab on a developer Mac, a nightly job, or a manually controlled macOS
runner with the same root-owned helper and isolated socket_vmnet network.

The default lab path sets `mihomo.redir_port` and `pf.redirect_tcp_to` to `0`.
The current Darwin mihomo build reports redir as unsupported, so transparent
TCP capture is covered by the TUN gate instead of PF TCP redirection.

The gateway binary intentionally does not receive a passwordless sudo rule.
Run `sudo -v` shortly before `make lab-test` so the test can use the cached sudo
credential without embedding or broadening root privileges.

The cached sudo credential is terminal-scoped and time-limited. If an agent or
automation runs `sudo -v` in one TTY and `make lab-test` in another, the lab
script may still fail its `sudo -n` preflight. Run `sudo -v` in the same
terminal session, immediately before the root-required lab target. The most
reliable form is `sudo -v && make <lab-target>`; revalidate for every gate in a
long session instead of treating one credential cache as permanent. A cold
`lab-up` can itself outlive the sudo ticket, so after it finishes run
`sudo -v && make lab-test...` again. If cleanup follows a long gate, also use
`sudo -v && make lab-down`; otherwise the VMs may stop while stale state for the
root-owned helper remains.
The real-profile IPv6 gate periodically runs `sudo -n -v` to refresh an
already valid ticket and stops that keepalive on exit; it never prompts for,
stores, or passes a password. If the ticket still expires, a failed gateway
stop makes the gate fail closed and retain its mode-`0600` recovery material.

### Common infrastructure failures

- In automatic-RA runs, `/etc/resolv.conf` may retain only an IPv4
  control/gateway resolver. IPv6 probes must fall back to the link-local next
  hop of the `omg0` IPv6 default route and add the `%omg0` scope. If an HTTP/3
  client evidence file is empty and the DNS fixture saw no query, inspect this
  prerequisite before diagnosing the QUIC data plane.
- quic-go may warn that the minimal guest could not raise its UDP receive
  buffer to the recommended size. When `CLIENT_IPV6_HTTP3_OK` follows, this is
  a throughput warning rather than a handshake failure; this functional gate
  does not claim QUIC performance coverage.
- Guest startup and cleanup restore the Lima control-plane DNS and make the
  local hostname resolvable through `/etc/hosts`. If provisioning reports
  `sudo: unable to resolve host` or still queries a stopped `192.168.50.1`, run
  `sudo /usr/local/bin/omg-lab-client restore-control` in the guest first.
- `networkctl status omg0` should name
  `/etc/systemd/network/05-open-mihomo-gateway-lab.network`. Duplicate IPv4 or
  IPv6 default routes mean netplan and another DHCP/RA client are competing for
  the interface. `renew6` accepts only a ULA that is neither `tentative` nor
  `dadfailed` and has a positive `preferred_lft`.
- A stale or unreachable proxy in `runtime/lab/proxy.env` can fail the patched
  Mihomo build before the data plane starts. An interrupted build can likewise
  leave the dedicated Go module cache incomplete. Inspect
  `runtime/lab/logs/mihomo-build.log` first; the script bypasses the stale proxy
  for Go mirrors and clears/retries once only for a confirmed missing file in
  that dedicated cache.
- A Go compile error immediately after `patching file` is a patched-Mihomo
  build-gate failure, not an IPv6 data-plane result. Run
  `scripts/build-opensurge-mihomo.sh` independently first. Any patch or pinned
  source revision change must pass an apply/build check against the pinned SHA.
- `sysctl ... operation not permitted` in an agent sandbox is an execution
  permission signal, not a data-plane result. Rerun the actual gate in the same
  approved PTY before drawing a host-network conclusion.
- VZ shutdown may print `use of closed network connection` in red. It is
  hostagent teardown noise, not a failure, when it is followed by both
  `has shut down` and `lab network stopped`.
- Review only the newly created `artifacts/lab/<timestamp>` for the current
  run. The IPv6 gate clears stale optional egress-fixture logs at startup so a
  historical hit cannot satisfy a new assertion.

The fixed-size Lima and mihomo downloads use segmented caches and verify the
checksum after assembly. If TLS or network instability interrupts an install,
rerun the same installer command: it reuses complete segments and resumes
incomplete ones. Do not copy unverified files into `runtime/tools/cache`.

The default client size is `1 CPU / 512 MiB`. A slow `lab-up` alone is not a
reason to increase it. Use `limactl shell <client> -- free -m`, `vmstat`, and
the guest OOM log to distinguish CPU or memory pressure from DNS, image
download, and apt provisioning waits. More CPU or memory will not fix a guest
that is mostly idle, still has available memory, and has no OOM evidence;
change the default only after sustained load, reclaim pressure, or OOM proves
that resources are the bottleneck.

The lab owns `192.168.50.1/22` only on its vmnet bridge. Do not leave the same
address on another interface. The real-device smoke also uses `192.168.50.1` on
interfaces such as `en7`; run `make real-device-stop` before `make lab-up`, or
remove the duplicate address with `sudo ifconfig <iface> inet 192.168.50.1
delete`. A duplicate LAN IP can make macOS route DNS responses to the wrong
interface and show up as a TUN lab DNS timeout.

## Commands

```sh
make lab-check    # show installed versions and network status
make lab-uninstall-root  # remove root-owned lab helper and sudoers rule
make lab-up       # create/start network and clients
make lab-status   # inspect host and client state
make lab-test     # run the end-to-end test and restore the host
make lab-test-tun # run the TUN transparent proxy gate
make lab-test-tun-imported-profile # run TUN with an imported profile fixture
make lab-test-tun-imported-egress  # switch TUN egress through a controlled proxy
make lab-test-tun-local-routing # prove local-Mac mode isolation
make lab-test-tun-device-policy # prove independent per-device TUN policies
make lab-tailscale-up # create/start the Tailnet peer (peer key needed once)
make lab-test-tailscale # prove peer IP, MagicDNS, TCP/UDP, and source authorization
make lab-tailscale-down # stop the Tailnet peer and preserve its identity
make lab-tailscale-destroy # delete local peer identity, not the admin-console record
make lab-test-ipv6-userspace # prove isolated-LAN IPv6 TCP/UDP takeover and withdrawal
make lab-test-ipv6-same-wifi # prove whole-LAN DHCP IPv6 and Medium RA
make lab-test-ipv6-same-lan # prove selective manual IPv6 without RA
make lab-test-ipv6-imported-egress # supplement with an actual profile and public IPv6
make lab-down     # stop clients and remove the host network
make lab-destroy  # delete the persistent Lima client disks too
```

Set `OMG_LAB_CLIENTS` to change the client names, or `OMG_LAB_TEST_URL` to use a
different HTTPS connectivity target.

## Safety

The generated config uses a vmnet-backed `bridge` interface and refuses to run
if that interface is also the default upstream. Never replace the lab interface
with `en0` or another normal LAN interface. `lab-up` also refuses to continue
when `192.168.50.1` is configured on a non-lab interface. `lab-test` always
attempts to stop the gateway and records diagnostics when an assertion fails.
