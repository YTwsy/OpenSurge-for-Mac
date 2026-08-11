[简体中文](#简体中文) · [English](#english)

## 简体中文

### 主要变化

- 新增首次网络配置建议：选择旁路由或局域网 DHCP 接管时，OpenSurge 会读取当前主网络并预填接口与 Mac IPv4；DHCP 接管还会建议避开 Mac、路由器和受保护地址的 `/24` 地址池，以及主路由网关与 DNS。建议只写入草稿，不会自动保存或修改 macOS 网络。
- 局域网 DHCP 接管新增按设备「IPv4 直连主路由」：固定 IPv4 仍由 OpenSurge 分配，但该设备续租后直接使用已配置的主路由网关与 DNS；IPv4 代理、设备规则和 OpenSurge 流量统计会暂停，切回 OpenSurge 后恢复。启用下游 IPv6 时，该设备经过 OpenSurge 的 IPv6 出站会被阻止；设备仍可能保留 SLAAC/RDNSS，主路由 RA 必须关闭或由 RA Guard 消除。
- 新增 Mihomo 独立自动恢复：进程消失会立即触发，本地 controller 连续两次拒绝连接后触发；每次未恢复事故最多自动尝试一次，并在连通性页保留手动兜底。配置、状态或 runtime 暂时不可观测时不会误判为健康，也不会清除这次事故的单次尝试保护。
- 菜单栏与 Web GUI 新增默认关闭、仅本次运行有效的「合盖保持运行」。退出 OpenSurge 或重启 Mac 后会恢复正常睡眠；若释放系统睡眠接管失败，Helper 会保留待释放标记并后台重试，同时继续提供其他安全操作。
- 诊断检查改为诊断页显式触发的 single-flight 后台任务，不再同步进入总览、菜单栏或 SSE 轮询热路径；配置变化会令旧结果失效。启动与重载仍执行各自的真实预检，不会复用缓存结果。
- 优化设备出口编辑：设备 ID、IPv4、MAC 与「按登记 IP 接入后生效」统一放入紧凑元数据区域；保存后，页面底部的浮动操作条会从「有未保存的设备修改」切换为「需重载」，不再另行显示顶部卡片。

### 选择安装包

| Mac 类型 | 安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

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

- mihomo 1.19.27 源码：<https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### Highlights

- Added first-run network suggestions: when selecting same-LAN manual gateway or DHCP takeover, OpenSurge reads the current primary network and pre-fills interfaces and the Mac IPv4. DHCP takeover also suggests a `/24` pool that avoids the Mac, router, and protected addresses, plus the upstream-router gateway and DNS. Suggestions remain an unsaved draft and never change macOS networking automatically.
- Added per-device **IPv4 direct via main router** for same-LAN DHCP takeover. OpenSurge still assigns the fixed IPv4, but after lease renewal that device uses the configured main-router gateway and DNS directly. IPv4 proxying, device rules, and OpenSurge traffic accounting pause until the device is switched back. When downstream IPv6 is enabled, IPv6 from that device is blocked on the OpenSurge packet path; the client may still retain SLAAC/RDNSS, so the main-router RA must be disabled or removed by RA Guard.
- Added narrow automatic Mihomo recovery: a missing process triggers immediately, while two consecutive local-controller connection refusals are required. Each unresolved incident gets at most one automatic attempt, with a manual fallback on Connectivity. Temporarily unreadable configuration, status, or runtime is treated as unknown rather than healthy and cannot clear the incident guard.
- Added **Keep Running with Lid Closed** to the menu bar and Web GUI. It is off by default and applies only to the current run; quitting OpenSurge or rebooting restores normal sleep. If releasing the system sleep override fails, the Helper retains a pending-release marker and retries in the background while continuing to serve other safe operations.
- Moved Doctor into an explicitly started, single-flight background task on Diagnostics instead of synchronously running it from dashboard, menu bar, or SSE polling paths. Configuration changes invalidate stale results. Start and reload still perform their own real preflight checks and never trust the cached Doctor result.
- Refined device editing: device ID, IPv4, MAC, and **Applies after the device connects with its registered IP** now share the compact metadata area. After saving, the floating bottom action bar changes from **Unsaved device changes** to **Reload required** instead of showing a separate card near the top.

### Choose a package

| Mac | Package | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

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

- mihomo 1.19.27 source: <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
