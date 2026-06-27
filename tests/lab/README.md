# Virtual LAN lab

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
- grants the current user passwordless sudo for only the helper's `start`,
  `stop`, and `status` commands.

The helper is copied to a root-owned path before the sudoers rule is installed.
The rule never executes scripts or binaries from this writable repository.
Run `make lab-uninstall-root` to remove the root-owned helper, socket_vmnet
copy, lab log, and sudoers rule.

Optional proxy variables can be stored in `runtime/lab/proxy.env`. The
installer and lab commands load that file for host-side operations. Lima VM
provisioning does not receive those proxy variables by default; set
`OMG_LAB_VM_PROXY=1` only when the proxy endpoint is reachable from inside the
VMs.

## Daily workflow

```sh
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-down
```

`lab-up` starts the DHCP-free host network and the two clients. `lab-test`
builds the current gateway, starts it with the generated lab config, renews both
client leases, checks routing, DNS, ICMP/NAT, direct HTTPS, and explicit HTTPS
through mihomo `mixed-port`, and then verifies cleanup. Artifacts are written
under `artifacts/lab`.

`lab-test-tun` is the TUN transparent proxy gate. It rewrites the lab config
with `transparent.mode: "tun"`, forwards dnsmasq to mihomo DNS, leaves the
clients without explicit proxy settings, and requires the no-proxy HTTPS
request to appear in `mihomo.log`.

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

## Commands

```sh
make lab-check    # show installed versions and network status
make lab-uninstall-root  # remove root-owned lab helper and sudoers rule
make lab-up       # create/start network and clients
make lab-status   # inspect host and client state
make lab-test     # run the end-to-end test and restore the host
make lab-test-tun # run the TUN transparent proxy gate
make lab-down     # stop clients and remove the host network
make lab-destroy  # delete the persistent Lima client disks too
```

Set `OMG_LAB_CLIENTS` to change the client names, or `OMG_LAB_TEST_URL` to use a
different HTTPS connectivity target.

## Safety

The generated config uses a vmnet-backed `bridge` interface and refuses to run
if that interface is also the default upstream. Never replace the lab interface
with `en0` or another normal LAN interface. `lab-test` always attempts to stop
the gateway and records diagnostics when an assertion fails.
