[简体中文](#简体中文) · [English](#english)

> **v0.2 系列代号：Wind Rose**<br>
> **v0.2 series codename: Wind Rose**

## 简体中文

### v0.2.4 主要变化（相对 v0.2.3）

v0.2.4 重点修复 Mac 本机的 DNS 与 IPv6 TUN 捕获路径，并补齐共享策略组切换节点后的连接刷新。下游设备仍使用各自的策略和 IPv6 packet 路径。

- **Mac 系统 DNS 随 TUN 协同**：在 TUN 自动路由就绪后，默认将上游网络服务的系统 DNS 设置到可被 mihomo 劫持的路径，减少本机查询绕过 TUN、连接失去域名或 fake-IP 上下文的情况。停止、回滚和代理引擎重启时按所有权恢复原设置，保留其他软件后续作出的修改；可在 Web GUI 中关闭。协同开启时，导入配置需使用显式解析器，避免 `system` 解析循环。
- **补全独立的 Mac IPv6 TUN 路由**：Mac 的系统 TUN 同时覆盖公网 IPv6、完整 fake IPv6 地址池和 Tailnet 精确路由，不再因下游 IPv6 接管开关或一条自定义精确路由而漏掉本机流量。本机规则仍只匹配系统 TUN 的精确身份，不改变下游设备的 IPv6 packet 路径。
- **共享策略组的连接刷新提示**：在「策略与节点健康」中成功切换普通策略组节点后，界面会说明新选择只影响后续连接，并邀请用户手动刷新。刷新按实际连接链中的策略组精确匹配，可涵盖 Mac、跟随网关规则或经其他策略组引用它的下游设备，同时保留不经过该组的连接；执行前会提示下载、通话等活动可能中断。切换节点本身不会自动关闭旧连接。

### 开发幕后：Team Cross

OpenSurge 的开发常要在 Mac 与多台下游设备上验证网络路径；调查和修复也可能跨越多台机器、多个 Agent Session，测试证据、关键判断和后续任务需要在这些会话之间持续传递。最初，我为自己的工作流做了一个多 Agent 协作工具。看到它在 OpenSurge 开发中显著提升了开发效率后，我意识到同样的痛点也存在于团队协作、产研协同等更广泛的场景。于是，我结合一线开发团队中积累的经验，围绕会话分享、证据讨论和任务交接重新设计这个工具，将它的能力扩展到真实的团队协作场景上；最终将它发展为独立的开源项目 [Team Cross](https://github.com/YTwsy/Team-Cross)。

Team Cross 让团队成员和各自的 Agent 预览并分享选定的 Codex、Claude Code 会话材料，在原文旁批注和引用；需要共同执行时，再明确开放访问，并交接或收回共享会话的输入权。在最近几个版本的 OpenSurge 开发中，它帮助我把多设备测试的上下文带入需求推进、问题排查和 PR 代码审查，减少反复解释背景与重新定位结论的工作，让这些任务更快接续和推进。

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

- OpenSurge-patched Mihomo `1.19.30-opensurge.1` 的上游基线源码：<https://github.com/MetaCubeX/mihomo/tree/ac017cdd246ce8bd547653d927e7bf77d7ee73d5>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### v0.2.4 highlights since v0.2.3

v0.2.4 focuses on the local Mac's DNS and IPv6 TUN capture paths and completes connection refresh after changing a shared policy group. Downstream devices keep their own policies and IPv6 packet path.

- **Mac system DNS coordination with TUN:** Once TUN auto-routing is ready, OpenSurge now sets the upstream network service's system DNS to a path Mihomo can intercept by default. This reduces local queries bypassing TUN and connections losing domain or fake-IP context. Stop, rollback, and engine restart restore the original setting according to ownership while preserving later changes by other software; the Web GUI can disable the feature. Imported profiles need explicit resolvers while it is enabled, avoiding a `system` resolver loop.
- **Complete, independent Mac IPv6 TUN routes:** The system TUN covers public IPv6, the full fake IPv6 pool, and precise Tailnet routes. Local capture no longer depends on downstream IPv6 takeover or disappears when a custom precise route is present. Mac-local rules still match only the system TUN's exact identity; downstream devices keep their separate IPv6 packet path.
- **Connection refresh for shared policy groups:** After a successful node change in Policies & Node Health, the UI explains that the new selection affects new connections and offers an explicit refresh. It matches the policy group's exact name in active connection chains, including connections from the Mac and downstream devices following gateway rules or referencing the group through another policy, while leaving unrelated connections alone. Possible interruptions to downloads or calls are disclosed before refresh; changing the node itself does not close existing connections.

### Behind the development: Team Cross

OpenSurge development often requires validating the same network path on a Mac and multiple downstream devices. Investigations and fixes can span several machines and Agent Sessions, so test evidence, key findings, and next steps need to travel between them. I initially built a multi-agent collaboration tool for my own workflow. Seeing how much it accelerated OpenSurge development made me realize that teams face the same challenges when working together across product and engineering. Drawing on my experience in development teams, I redesigned and rebuilt the tool around sharing sessions, discussing evidence, and handing off tasks. What began as a personal tool became the independent open-source project [Team Cross](https://github.com/YTwsy/Team-Cross).

Team Cross lets teammates and their agents preview and share selected Codex and Claude Code session material, annotate and cite passages alongside the original, and explicitly grant access when they need to work together. They can hand over or reclaim input control of a shared session. In recent OpenSurge versions, it has helped me carry multi-device test context into feature development, troubleshooting, and PR code review. That reduces repeated explanations and the need to rediscover conclusions, helping these tasks continue and move forward faster.

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

- Upstream baseline source for OpenSurge-patched Mihomo `1.19.30-opensurge.1`: <https://github.com/MetaCubeX/mihomo/tree/ac017cdd246ce8bd547653d927e7bf77d7ee73d5>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
