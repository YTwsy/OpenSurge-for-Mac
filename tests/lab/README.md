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
vmnet host network (192.168.50.0/24, no platform DHCP)
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
the internal groups.

`lab-test-tun-device-policy` uses both clients as independently identified LAN
devices. It assigns them fixed `.101` and `.102` DHCP leases, proves one
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
The passing artifact retains that applied snapshot, runtime state, generated
dnsmasq/mihomo configuration, and the initial and post-reload device views so
the boundary remains auditable.
Rule/template/provider compilation stays in unit tests.

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

The lab owns `192.168.50.1/24` only on its vmnet bridge. Do not leave the same
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
