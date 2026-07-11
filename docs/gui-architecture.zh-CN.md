# Web GUI 与菜单栏 App

OpenSurge 现在提供同一个本地控制面之上的两个 GUI 入口：

- `cmd/opensurge-control` 是只监听 `127.0.0.1` 的 Go Control API，并嵌入
  `web/` 构建出的 React 应用；
- `apps/menubar/` 是 macOS 13+ 的原生 SwiftUI `MenuBarExtra`，只显示状态、
  恢复警报并打开 Web GUI；
- `cmd/opensurge-helper` 是窄权限 Unix-socket helper，只接受 start/stop、网络
  固定地址/恢复 DHCP 和主动 DHCP OFFER 探测等固定动作，不提供任意 shell 命令；
  生产配置、runtime 和可执行文件必须位于允许目录、由 root 拥有且不能被普通用户修改。

## 开发运行

```sh
make web-install
make control-build
./bin/opensurge-control --config examples/config.example.yaml
```

控制服务会输出一个 30 秒内有效、只能使用一次的 bootstrap URL。浏览器通过它换取
`HttpOnly`、`SameSite=Strict` session cookie。API mutation 还会校验 Origin；菜单栏
App 使用应用支持目录内权限为 `0600` 的本地 token 请求一个新的短期 bootstrap URL，
不会把长期凭据放进浏览器历史。

Control API 默认位置是 `http://127.0.0.1:61767`。端点描述、token、来源快照、操作记录
和 same-WiFi 恢复状态位于：

```text
~/Library/Application Support/OpenSurge/
```

开发环境若确实需要执行网关动作，可用 root 身份运行 control service 并显式传入
`--direct-root`。普通开发和 UI 验证不要使用这个参数；未安装 helper 时 start/stop
会返回结构化错误，不会尝试提权或弹出不透明的 shell。

## Web 页面

Web GUI 包含总览、网络设置、来源、设备、策略和诊断。来源支持本地 YAML 与 HTTPS
URL，先保存 SHA-256 标识的只读快照，再检查 profile inventory 和保留命名空间；应用
时使用真实 mihomo `-t` 验证 overlay。URL 获取拒绝 loopback、私网、链路本地地址、
非 HTTPS 重定向、超过三次重定向和超过 10 MiB 的响应。

同一 source 的刷新保留旧版本 metadata、digest、inventory 和 applied 标记，新内容保持
为未应用草稿，并展示 proxies/groups/providers 与规则数 diff；只有显式点击应用并通过
revision 与第二次 mihomo 校验后才替换运行版本。

带 token/query/basic-auth 的完整刷新 URL 存入 macOS Keychain，不进入 sources JSON、
API 响应或界面；公开 `origin` 会移除 userinfo、query 和 fragment。来源快照仍是用户目录
下权限为 `0600` 的按 digest 版本文件。

`GET/PUT /api/v1/config` 只暴露 topology、DHCP/DNS、TUN 与 device-policy 开关等
非敏感字段，并强制 `If-Match` revision。生产环境由 helper 原子写入 root-owned config；
网关运行或恢复未完成时拒绝 topology 修改。网络页可切换 `same_lan`、
`same_wifi_dhcp`、`isolated_lan`，并可初始化空 device-policy 文件。

设备 selector 使用已应用的 device-policy bundle；API 会根据设备 ID 和 slot 重建
`device/<id>/<slot>`，不会接受调用方直接伪造任意 group 名。设备、模板或规则编辑是
desired 配置变更；已有 selector 成员切换是 live operation。
编辑器覆盖 profile、template、本地或 HTTPS rule-set、domain/CIDR/protocol/port、action
和 selector。helper 保存前会用当前 imported inventory 与真实 mihomo 校验候选，不只做
JSON 结构检查。

`/api/v1/events` 每两秒观察 config、gateway、desired/applied digest 与 recovery，只有
状态变化时发送 `state` SSE，另有 15 秒 heartbeat。诊断页通过受认证接口显示 live
connections 与最多 80 行近期日志；已知 mihomo/upstream 凭据在 API 返回前脱敏。

## same-WiFi 恢复状态

`/api/v1/recovery` 持久化以下受验证状态机：

```text
idle -> prepared -> mac_static -> router_dhcp_disabled_confirmed
     -> gateway_active -> gateway_stopped_waiting_router_dhcp
     -> router_dhcp_restored -> complete
```

当配置为 `same_wifi_dhcp` 时，Control API 在没有
`router_dhcp_disabled_confirmed` 收据时拒绝 start。确认收据来自 root helper 的主动
DHCPDISCOVER：仍收到任何 OFFER 就硬阻塞。成功 stop 后状态进入
`gateway_stopped_waiting_router_dhcp`，只有重新探测到路由器 OFFER 后才允许把 Mac 恢复
为自动 DHCP；菜单栏和 Web GUI 会持续显示恢复警报。状态机不会自动修改未知路由器，
也不能把同一 Wi-Fi 描述为不可绕过隔离。

## 菜单栏边界

菜单栏 App 不提供 start/stop、provider refresh 或策略切换。它每 15 秒获取一次
`/api/v1/menubar`，窗口打开时每 2 秒刷新，失败时指数退避到最多 60 秒，并根据 stopped、running、degraded、
recovery、unreachable 使用不同的 SF Symbol。恢复警报优先于其他状态。

构建：

```sh
make menubar-build
./scripts/build-menubar-app.sh
```

构建统一 pkg：

```sh
OPENSURGE_MIHOMO_BINARY=/path/to/mihomo \
OPENSURGE_DNSMASQ_BINARY=/path/to/dnsmasq \
make gui-installer
```

安装包包含 Web 静态资源（嵌入 control binary）、用户级 Control Service、菜单栏 App、
CLI 和 root helper。postinstall 会创建 root:admin、用户只读的 applied 配置/runtime，
安装固定 launchd 服务；卸载脚本在 recovery 未完成时拒绝删除，并先停止网关。

正式发布可设置 `OPENSURGE_CODESIGN_IDENTITY` 和 `OPENSURGE_INSTALLER_IDENTITY` 后构建，
再设置已由 `notarytool store-credentials` 创建的 `OPENSURGE_NOTARY_PROFILE`：

```sh
make gui-notarize PKG=artifacts/gui-installer/OpenSurge-for-Mac-0.1.0.pkg
```

缺少 Apple Developer 身份时只能产出明确标记的未签名本地测试 pkg，不能宣称已经签名或
notarize。当前安装器 seed 配置必须使用 managed profile 且不引用外部 device-policy；
订阅与设备策略在安装后经控制面导入，避免 pkg 携带工作区绝对路径。

## 验证

```sh
make test
make web-build
make menubar-build
```

修改真实网关、TUN、DHCP 或设备策略数据面后，仍须运行对应的 lab/same-WiFi gate；
GUI 构建通过不能替代这些 host-network 证据。
