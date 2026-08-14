[简体中文](#简体中文) · [English](#english)

> **v0.2 系列代号：Wind Rose**<br>
> **v0.2 series codename: Wind Rose**

## 简体中文

### 相对 v0.1.25 的主要变化

- 新增实验性的**下游 IPv6 接管**。OpenSurge 现在可在独立下游 LAN、局域网 DHCP 接管和旁路由三种拓扑中，为选定客户端建立 IPv6 地址、DNS、透明数据面和按设备策略；下游 IPv6 不进入 macOS 系统 TUN，而由 macOS BPF packet broker 送入本项目补丁构建的 Mihomo `opensurge-packet`/gVisor 数据面，覆盖 TCP 与 UDP，并保留 source MAC 用于设备身份和独立出口策略。
- Web GUI 新增完整的下游 IPv6 配置与拓扑说明：`dns.ipv6` 独立控制 AAAA/fake IPv6，`transparent.tun_ipv6` 提供 `off / auto / always`；共享二层网络必须显式确认已消除主路由 RA/DHCPv6 或使用 RA Guard，旁路由模式则提供手工 ULA、link-local 网关和 DNS 速查。
- 新增首次网络配置建议：选择旁路由或局域网 DHCP 接管时，OpenSurge 会读取当前主网络并预填接口与 Mac IPv4；DHCP 接管还会建议避开 Mac、路由器和受保护地址的 `/24` 地址池，以及主路由网关与 DNS。建议只写入草稿，不会自动保存或修改 macOS 网络。
- 来源页新增受保护的本地快照操作：每个已导入来源都会显示 OpenSurge 管理的快照位置，并可复制完整路径、在 Finder 中选中，或导出独立的可编辑 YAML。执行文件动作前会重新校验 source ID、digest、规范化管理路径、私有权限和内容摘要；导出副本以 `0600` 权限写入 `~/Library/Application Support/OpenSurge/exports/`，不会覆盖旧文件，也不会被后续来源刷新覆盖。修改后的副本需要重新按本地文件导入，才会成为新草稿。侧栏状态区现在还会显示构建时写入的实际 Release tag 与 Wind Rose 代号，而不是泛化的 `v0.2` 标识。
- 局域网 DHCP 接管新增按设备**IPv4 直连主路由**：固定 IPv4 仍由 OpenSurge 分配，但该设备续租后直接使用已配置的主路由网关与 DNS；IPv4 代理、设备规则和 OpenSurge 流量统计会暂停，切回 OpenSurge 后恢复。启用下游 IPv6 时，该设备经过 OpenSurge packet path 的 IPv6 出站会被最高优先级阻止；设备仍可能保留 SLAAC/RDNSS，因此主路由 RA 必须关闭或由 RA Guard 消除。
- 新增窄范围的 Mihomo 自动恢复与更清晰的运行操作反馈：进程消失会立即触发恢复，本地 controller 连续两次拒绝连接后触发；每次未恢复事故最多自动尝试一次，连通性页保留手动兜底。配置、状态或 runtime 暂时不可观测时不会误判为健康，Web GUI 会持续显示启动、重载、恢复和失败结果。
- 菜单栏与 Web GUI 新增默认关闭、仅本次运行有效的**合盖保持运行**。退出 OpenSurge 或重启 Mac 后会恢复正常睡眠；若释放系统睡眠接管失败，Helper 会保留待释放标记并后台重试，同时继续提供其他安全操作。
- 诊断检查改为诊断页显式触发的 single-flight 后台任务，不再同步进入总览、菜单栏或 SSE 轮询热路径；配置变化会令旧结果失效。设备出口编辑也得到简化，保存后的底部操作条会明确提示需要重载。

### IPv6 设计详解

#### 目标与双数据面

这项能力解决的是**下游设备 IPv6 的透明接入、分流和设备身份**，不是把运营商分配的公网前缀原样桥接或透传给客户端。OpenSurge 保留两条入口机制不同、但共享 Mihomo 规则和 outbound 的透明路径：

- 下游 IPv4 与 Mac 本机透明代理继续使用 Mihomo TUN；
- 下游 IPv6 从物理 Ethernet 接入 BPF packet path，不进入 macOS 系统 utun。

```text
下游 Ethernet IPv6 frame + source MAC
              ↓
macOS BPF packet broker
              ↓  mode 0600 Unix datagram
OpenSurge-patched Mihomo opensurge-packet listener
              ↓
gVisor TCP / UDP
              ↓
IN-USER(device:<id>) → 设备规则 → outbound
```

返回包沿相反路径写回下游 Ethernet。QUIC 由 UDP/443 carrier 承载；这不等于支持任意 IPv6 协议，也不把受控 QUIC 形态测试描述成完整 HTTP/3 握手。三种拓扑仍要求 `transparent.mode: "tun"`，但不会重新启用 `redir-port` 或 PF TCP redirect。

#### OpenSurge 对 Mihomo 的修改

v0.2.0 安装包不再分发未经修改的上游 Mihomo 二进制，而是从固定的上游 Mihomo `1.19.27` 源码提交构建 `1.19.27-opensurge.1`。构建过程校验源码归档 SHA-256，应用仓库内公开的 `patches/mihomo`，并为 Apple Silicon 和 Intel 分别使用 `with_gvisor` 构建；补丁、对应上游源码和构建步骤均公开且固定，可供审计和重新构建。

修改范围保持在下游 IPv6 packet ingress：

- 新增 `type: opensurge-packet` inbound，以及 socket、MTU 和 MAC→设备用户映射配置；
- 新增版本化的 `OS6P` Unix datagram 协议，在 mode `0600` socket 上携带原始 IPv6 L3 packet 和 6-byte source MAC；
- 将 broker 送入的 IPv6 packet 注入 Mihomo 已有的 sing-tun/gVisor TCP/UDP 栈，并把观察到的 MAC 映射为 `IN-USER(device:<id>)`，从而复用既有规则、代理组和 outbound；
- 同一个 IPv6 source 若被不同 MAC 使用会拒绝后来的冲突映射，不会静默改绑；
- 当操作者显式启用 IPv6 DNS 或 `opensurge-packet` listener 时，即使 macOS 宿主机没有原生 IPv6 地址，也不会让 Mihomo 的宿主能力探测错误关闭这条用户态 IPv6 路径。

我们没有为此改写 Mihomo 的节点协议、订阅/profile 模型、规则引擎、代理组或 outbound 实现。`opensurge-packet` 是 OpenSurge 的 macOS BPF broker 与 Mihomo 现有数据面之间的一条项目专用入口；它不是上游官方 Mihomo listener，安装包中的二进制也不应描述成未经修改的上游构建。

#### 两个独立开关

| 设置 | 作用 |
| --- | --- |
| `dns.ipv6` | 控制 OpenSurge DNS 是否回答 AAAA，并生成 fake IPv6。它本身不会发布 RA 或建立下游透明路由。 |
| `transparent.tun_ipv6: off` | 不建立 OpenSurge 下游 IPv6。 |
| `transparent.tun_ipv6: auto` | 仅当所选上游接口同时具有公网全局 IPv6 地址（排除 ULA）和 IPv6 默认路由时建立下游路径。 |
| `transparent.tun_ipv6: always` | 无论上游是否有原生 IPv6，都建立下游 RA/手工接入和用户态 packet path。 |

`always` 不会凭空提供公网 IPv6。没有原生上游 IPv6 时，fake IPv6 域名目标仍可由支持相应流量的代理出口承载；`DIRECT` 到真实公网 IPv6 地址会因缺少上游路由而失败，HTTP-only 代理也不能承载 UDP/QUIC。

#### 三种拓扑

| 拓扑 | IPv6 接入方式 | 必要条件 |
| --- | --- | --- |
| 独立下游 LAN | 自动发布 RA/SLAAC/RDNSS | 独立 AP、SSID 或 VLAN；OpenSurge 直接控制下游二层。 |
| 局域网 DHCP 接管 | 自动发布 RA/SLAAC/RDNSS，使用标准 Medium 默认路由器优先级 | 必须关闭主路由 IPv6 RA/DHCPv6，或使用 RA Guard 保证 OpenSurge 是唯一默认路由提供者，并显式确认 shared-L2 readiness。 |
| 旁路由 | 不广播 RA；客户端手工填写唯一 ULA、Mac 接口的 link-local 默认网关和 link-local DNS | 需要移除该设备原有的主路由 IPv6 默认路由，避免绕过 OpenSurge。 |

共享二层网络的正确性来自**消除竞争 RA**，而不是让 OpenSurge 用更高优先级压过主路由。控制面会区分 Mac 接收自身 RA 后形成的自有默认路由与外部路由器发布的竞争默认路由；只存在自有路由时不会误报绕过风险。

#### 地址规划

| 用途 | 地址或前缀 |
| --- | --- |
| Mihomo fake IPv6 | `fdfe:dcba:9876::/64` |
| Mihomo 系统 TUN | `fdfe:dcba:9877::1/126` |
| 下游 SLAAC / 手工 ULA | `fdfe:dcba:9878::/64` |
| Mac 下游 gateway alias / DNS listener | `fdfe:dcba:9878::1` |

自动模式向客户端发布下游接口的 link-local 地址作为 RDNSS。旁路由客户端的默认网关和 DNS 也都填写 Mac 下游接口的 link-local 地址；`fdfe:dcba:9878::1` 不是旁路由客户端的填写契约。

#### 设备身份、策略与 IPv4 主路由绕行

BPF broker 保留 source MAC，Mihomo listener 将其映射为内部 `device:<id>`，再复用已有设备规则和 selector。这个身份用于流量归属和分流，不是防 MAC spoofing 的安全认证；IPv6 地址与 MAC 发生冲突时保持 fail closed。

局域网 DHCP 接管中的“IPv4 直连主路由”只绕行 IPv4。开启下游 IPv6 后，OpenSurge 仍保留该设备的 MAC 映射，但只用于最前置的 `TUN + IN-USER → REJECT`；不会恢复普通设备规则、selector 或 IPv4 流量统计。若主路由仍发布 RA，客户端可能从 OpenSurge packet path 之外直接走 IPv6，因此不能把这一策略描述成共享二层上的绝对按设备 IPv6 隔离。

#### 生命周期、回滚与当前验证边界

自动模式按 **Mihomo listener → BPF broker → gateway alias → dnsmasq RA** 启动；停止时按 **dnsmasq → lifetime-zero RA 撤销 → alias → broker → Mihomo** 清理。旁路由模式使用相同 packet-path 生命周期，但不发送启动或撤销 RA。中间失败会进入统一 rollback，并保留未完成清理所需的 runtime state。

仓库为三种拓扑分别提供可复现的 host-network IPv6 Lab 门槛，覆盖双客户端、Medium RA 或无 RA 手工接入、TCP、受控 UDP、QUIC 形态 UDP carrier、MAC/InUser 设备策略和停止清理。2026-08-13 的物理下游 Mac 补充 smoke 还验证了强制 IPv6 HTTPS、BPF 双向收发和设备 selector；同一观察窗口的浏览器流量表现为 IPv6 TCP/443 与 IPv4 UDP/443 混合选择。

这些证据不等同于物理设备 IPv6 UDP/QUIC、原生公网 IPv6、竞争 RA、停止时 RA 撤销、睡眠/重启恢复或所有设备类型都已验收。

### 选择安装包

| Mac 类型 | 安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> 安装包未进行 Developer ID 签名或 notarization。正式 Release 会同时提供 `SHA256SUMS` 和 GitHub build provenance，供下载后核验。

### 安装

1. 下载与你的 Mac 芯片匹配的安装包。
2. 双击安装包。如果 macOS 阻止打开，请进入**系统设置 → 隐私与安全性**，选择**仍要打开**并完成身份验证，然后重新打开安装包。
3. 安装完成后，从 `/Applications` 打开 **OpenSurge**。

安装完成后，网关默认保持停止；只有在 OpenSurge 控制面中明确操作后才会启动。

<details>
<summary>可选：校验下载文件</summary>

下载 `SHA256SUMS`，运行 `shasum -a 256 安装包名称`，并与文件中的对应记录比较。

也可以使用 GitHub CLI 核对安装包的构建来源：

```sh
gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
```

Intel 安装包请将命令中的 `arm64` 替换为 `x86_64`。

</details>

### 许可证

OpenSurge 自有代码采用 `GPL-3.0-only`。第三方许可证、声明与准确的对应源码链接会安装到：

`/Library/Application Support/OpenSurge/share/licenses/`

- OpenSurge-patched Mihomo `1.19.27-opensurge.1` 的上游基线源码：<https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### Highlights since v0.1.25

- Added experimental **downstream IPv6 takeover**. OpenSurge can now establish IPv6 addressing, DNS, a transparent data plane, and per-device policy for selected clients across isolated downstream LAN, same-LAN DHCP takeover, and bypass-router topologies. Downstream IPv6 does not enter a macOS system TUN: a macOS BPF packet broker feeds the OpenSurge-patched Mihomo `opensurge-packet`/gVisor data plane, preserving the source MAC for device identity and independent egress policy while carrying TCP and UDP.
- Added complete downstream IPv6 controls and topology guidance to the Web GUI. `dns.ipv6` independently controls AAAA/fake IPv6, while `transparent.tun_ipv6` offers `off / auto / always`. Shared-L2 networks require explicit confirmation that main-router RA/DHCPv6 has been removed or blocked with RA Guard; bypass-router mode provides a manual ULA, link-local gateway, and DNS reference.
- Added first-run network suggestions. When selecting same-LAN bypass-router mode or DHCP takeover, OpenSurge reads the current primary network and pre-fills interfaces and the Mac IPv4. DHCP takeover also suggests a `/24` pool that avoids the Mac, router, and protected addresses, plus the main-router gateway and DNS. Suggestions remain an unsaved draft and never change macOS networking automatically.
- Added protected local-snapshot actions to Sources. Each imported source shows its OpenSurge-managed snapshot location and can copy the full path, reveal the file in Finder, or export a separate editable YAML. Before any file action, OpenSurge revalidates the source ID, digest, canonical managed path, private permissions, and content hash. Exports are written with mode `0600` under `~/Library/Application Support/OpenSurge/exports/`, never overwrite an older file, and are not replaced by later source refreshes. An edited copy must be imported again as a local file before it becomes a new draft. The sidebar status area now also shows the build-stamped Release tag with the Wind Rose codename instead of a generic `v0.2` label.
- Added per-device **IPv4 direct via main router** for same-LAN DHCP takeover. OpenSurge still assigns the fixed IPv4, but after lease renewal the device uses the configured main-router gateway and DNS directly. IPv4 proxying, device rules, and OpenSurge traffic accounting pause until the device is switched back. When downstream IPv6 is enabled, IPv6 from that device is rejected at the highest priority on the OpenSurge packet path; the client may still retain SLAAC/RDNSS, so main-router RA must be disabled or removed by RA Guard.
- Added narrow automatic Mihomo recovery and clearer operation feedback. A missing process triggers recovery immediately, while two consecutive local-controller connection refusals are required. Each unresolved incident gets at most one automatic attempt, with a manual fallback on Connectivity. Temporarily unreadable configuration, status, or runtime is treated as unknown rather than healthy, and the Web GUI continues to report start, reload, recovery, and failure results.
- Added **Keep Running with Lid Closed** to the menu bar and Web GUI. It is off by default and applies only to the current run; quitting OpenSurge or rebooting restores normal sleep. If releasing the system sleep override fails, the Helper retains a pending-release marker and retries in the background while continuing to serve other safe operations.
- Moved Doctor into an explicitly started, single-flight background task on Diagnostics instead of synchronously running it from dashboard, menu bar, or SSE polling paths. Configuration changes invalidate stale results. Device egress editing is also more compact, and the bottom action bar now clearly reports when a reload is required.

### IPv6 design in detail

#### Goal and two transparent data planes

This feature provides **transparent downstream-device IPv6 onboarding, routing policy, and device identity**. It is not native bridging or pass-through of an ISP-delegated public prefix. OpenSurge retains two ingress mechanisms that share Mihomo rules and outbounds:

- downstream IPv4 and local-Mac transparent proxying continue to use Mihomo TUN;
- downstream IPv6 enters from physical Ethernet through the BPF packet path and never enters a macOS system utun.

```text
downstream Ethernet IPv6 frame + source MAC
              ↓
macOS BPF packet broker
              ↓  mode-0600 Unix datagram
OpenSurge-patched Mihomo opensurge-packet listener
              ↓
gVisor TCP / UDP
              ↓
IN-USER(device:<id>) → device rules → outbound
```

Replies travel back through the reverse path to downstream Ethernet. QUIC is carried as UDP/443; that does not imply support for every IPv6 protocol, and a controlled QUIC-shaped carrier test is not a full HTTP/3 handshake. All three topologies still require `transparent.mode: "tun"`, but this design does not re-enable `redir-port` or PF TCP redirection.

#### OpenSurge modifications to Mihomo

The v0.2.0 installers no longer distribute an unmodified upstream Mihomo binary. They build `1.19.27-opensurge.1` from a pinned upstream Mihomo `1.19.27` source commit. The build verifies the source-archive SHA-256, applies the public `patches/mihomo` tree in this repository, and compiles separate Apple Silicon and Intel binaries with `with_gvisor`; the patch, corresponding upstream source, and source-pinned build steps remain available for audit and rebuilding.

The modification is narrowly scoped to downstream IPv6 packet ingress:

- adds a `type: opensurge-packet` inbound with socket, MTU, and MAC-to-device-user configuration;
- adds the versioned `OS6P` Unix datagram protocol, carrying a raw IPv6 L3 packet and six-byte source MAC over a mode-`0600` socket;
- injects broker-delivered IPv6 packets into Mihomo's existing sing-tun/gVisor TCP/UDP stack and maps the observed MAC to `IN-USER(device:<id>)`, reusing existing rules, proxy groups, and outbounds;
- rejects a later conflicting MAC mapping for the same IPv6 source instead of silently rebinding it;
- keeps the explicit IPv6 DNS or `opensurge-packet` data path enabled even when macOS host capability detection finds no native IPv6 address.

This work does not rewrite Mihomo's node protocols, subscription/profile model, rule engine, proxy groups, or outbound implementations. `opensurge-packet` is a project-specific ingress between the OpenSurge macOS BPF broker and Mihomo's existing data plane; it is not an official upstream Mihomo listener, and the packaged binary must not be described as an unmodified upstream build.

#### Two independent controls

| Setting | Behavior |
| --- | --- |
| `dns.ipv6` | Controls whether OpenSurge DNS answers AAAA and generates fake IPv6. It does not publish RA or establish downstream transparent routing by itself. |
| `transparent.tun_ipv6: off` | Does not establish OpenSurge downstream IPv6. |
| `transparent.tun_ipv6: auto` | Establishes the downstream path only when the selected upstream interface has both a public global IPv6 address (excluding ULA) and an IPv6 default route. |
| `transparent.tun_ipv6: always` | Establishes downstream RA/manual onboarding and the userspace packet path whether or not native upstream IPv6 is present. |

`always` does not create public IPv6 connectivity by itself. Without native upstream IPv6, fake-IPv6 domain targets can still use a proxy outbound that supports the corresponding traffic; `DIRECT` to a literal public IPv6 address fails without an upstream route, and an HTTP-only proxy cannot carry UDP/QUIC.

#### Three topologies

| Topology | IPv6 onboarding | Requirement |
| --- | --- | --- |
| Isolated downstream LAN | Publishes RA/SLAAC/RDNSS automatically | An isolated AP, SSID, or VLAN where OpenSurge controls the downstream L2. |
| Same-LAN DHCP takeover | Publishes RA/SLAAC/RDNSS automatically with the standard Medium router preference | Disable main-router IPv6 RA/DHCPv6 or use RA Guard so OpenSurge is the only default-router provider, then explicitly confirm shared-L2 readiness. |
| Bypass-router | Sends no RA; the client uses a unique manual ULA plus the Mac interface's link-local default gateway and link-local DNS | Remove that client's existing main-router IPv6 default route so it cannot bypass OpenSurge. |

Correctness on shared L2 comes from **eliminating competing RA**, not from assigning OpenSurge a higher router preference. The control plane distinguishes a self-owned default route created when the Mac receives its own RA from an external competing route, so a self-only route no longer produces a false bypass warning.

#### Address plan

| Purpose | Address or prefix |
| --- | --- |
| Mihomo fake IPv6 | `fdfe:dcba:9876::/64` |
| Mihomo system TUN | `fdfe:dcba:9877::1/126` |
| Downstream SLAAC / manual ULA | `fdfe:dcba:9878::/64` |
| Mac downstream gateway alias / DNS listener | `fdfe:dcba:9878::1` |

Automatic modes advertise the downstream interface's link-local address as RDNSS. A bypass-router client also uses the Mac downstream interface's link-local address for both its default gateway and DNS; `fdfe:dcba:9878::1` is not the bypass-router client configuration contract.

#### Device identity, policy, and main-router IPv4 bypass

The BPF broker preserves the source MAC, and the Mihomo listener maps it to the internal `device:<id>` identity before reusing existing device rules and selectors. This identity is for traffic attribution and routing policy, not security authentication against MAC spoofing. IPv6 address/MAC conflicts remain fail closed.

**IPv4 direct via main router** in same-LAN DHCP takeover bypasses only IPv4. When downstream IPv6 is enabled, OpenSurge retains the device's MAC mapping solely for the earliest `TUN + IN-USER → REJECT`; it does not restore normal device rules, selectors, or IPv4 traffic accounting. If the main router still advertises RA, the client may use IPv6 outside the OpenSurge packet path, so this must not be described as absolute per-device IPv6 isolation on uncontrolled shared L2.

#### Lifecycle, rollback, and current validation boundary

Automatic modes start in **Mihomo listener → BPF broker → gateway alias → dnsmasq RA** order. Stop runs **dnsmasq → lifetime-zero RA withdrawal → alias → broker → Mihomo**. Bypass-router mode uses the same packet-path lifecycle without startup or withdrawal RA. Intermediate failures enter the common rollback path and retain runtime state needed to retry incomplete cleanup.

The repository provides reproducible host-network IPv6 Lab gates for all three topologies, covering two clients, Medium RA or manual no-RA onboarding, TCP, controlled UDP, a QUIC-shaped UDP carrier, MAC/InUser device policy, and stop cleanup. A physical downstream-Mac smoke on 2026-08-13 additionally verified forced IPv6 HTTPS, bidirectional BPF traffic, and a device selector; browser traffic in the same observation window mixed IPv6 TCP/443 with IPv4 UDP/443.

This evidence does not establish physical-device IPv6 UDP/QUIC, native public IPv6, competing-RA handling, stop-time RA withdrawal, sleep/reboot recovery, or broad device compatibility.

### Choose a package

| Mac | Package | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> The installers are not Developer ID signed or notarized. The stable Release will also provide `SHA256SUMS` and GitHub build provenance for post-download verification.

### Install

1. Download the package matching your Mac.
2. Double-click the package. If macOS blocks it, open **System Settings → Privacy & Security**, choose **Open Anyway**, authenticate, and reopen the package.
3. After installation, open **OpenSurge** from `/Applications`.

The gateway remains stopped after installation and starts only when explicitly requested from the OpenSurge control plane.

<details>
<summary>Optional: verify the download</summary>

Download `SHA256SUMS`, run `shasum -a 256 PACKAGE_NAME`, and compare the result with the corresponding entry.

You can also verify the package's GitHub build provenance:

```sh
gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
```

For the Intel package, replace `arm64` with `x86_64`.

</details>

### License

OpenSurge original code is licensed under `GPL-3.0-only`. Third-party license texts, notices, and exact corresponding-source links are installed under:

`/Library/Application Support/OpenSurge/share/licenses/`

- Upstream baseline source for OpenSurge-patched Mihomo `1.19.27-opensurge.1`: <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
