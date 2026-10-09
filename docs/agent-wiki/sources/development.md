# 控制面与桌面开发

本页维护本地开发、调试方式与构建入口。界面职责见 [GUI 概念](../wiki/concepts/gui-control-plane.md)，运行与安全契约见 [Control API](control-api.md) 和 [桌面宿主](decisions/desktop-host.md)。

## 开发运行

```sh
make web-install
make control-build
./bin/opensurge-control --config examples/config.example.yaml
```

控制服务会输出一个 30 秒有效、一次性的 bootstrap URL，浏览器用它换取 HttpOnly
session。Wails 通过 native relay 执行同样的交换，token 和 cookie 保留在原生内存，不进入
JavaScript。正式安装位置的 App 启动时唤醒已有用户 Control Service；显式重新连接同样
不使用 `kickstart -k`，不会强制重启后台或启动网关。Preview 启动不操作服务。

### 选择嵌入式调试或 Vite 热更新

需要验证正式构建、认证、路由和 API 交互时，使用嵌入式 Web GUI。为避免与已安装的
Control Service 默认端口 `61767` 冲突，可以让仓库内的开发实例监听独立端口，并把其
session、来源、操作记录和恢复状态隔离到临时 store：

```sh
make control-build
./bin/opensurge-control \
  --config examples/config.example.yaml \
  --addr 127.0.0.1:61768 \
  --store /private/tmp/opensurge-control-dev
```

在 30 秒内打开终端输出的一次性 URL。`web/` 的构建产物会写入
`internal/webui/dist` 并嵌入 Control Service 二进制，因此修改前端源码后需要停止开发
实例，重新运行 `make control-build`，再打开新生成的 URL。

只调布局、样式和读取状态时，可以使用 Vite 热更新：

```sh
cd web
pnpm dev
```

Vite 监听 `http://127.0.0.1:5173`，并把 `/api` 和 `/bootstrap` 代理到固定的
`http://127.0.0.1:61767`。如果 `61767` 没有已安装的 Control Service，可以在另一个
终端启动使用示例配置的开发后端：

```sh
./bin/opensurge-control \
  --config examples/config.example.yaml \
  --addr 127.0.0.1:61767 \
  --store /private/tmp/opensurge-control-dev
```

不要先打开它输出的原始 bootstrap URL；把 URL 中的端口从 `61767` 改为 `5173` 后再
打开，使一次性 code 经 Vite 代理换取属于 `127.0.0.1` 的 session cookie。例如：

```text
http://127.0.0.1:5173/bootstrap?code=...
```

如果已安装的 Control Service 正在 `61767` 运行，则不要再启动第二个后端。先从菜单栏
打开一次正式 Web GUI 取得 session，再访问 `http://127.0.0.1:5173/dashboard`；浏览器
会把同一 `127.0.0.1` 主机的 cookie 发给 Vite，代理后的只读 API 请求因此可以展示当前
安装实例的真实数据。遇到 `401` 时，从菜单栏重新打开正式 Web GUI，再刷新开发页面。

当前 Vite 开发源是 `http://127.0.0.1:5173`，而 Control Service 只接受与自身 base URL
一致的浏览器 mutation Origin。因此 Vite 模式适合热更新和读取真实状态，但保存配置、
切换策略、启停网关等写操作会返回 `origin_rejected`。需要调试这些交互时使用上面的
嵌入式页面；普通 UI 验证不要为了绕过此限制使用 `--direct-root`。

### 开发页面的数据来源

Web GUI 没有独立的运行时 mock/demo 模式。页面始终读取它所连接的 Control Service；
Vitest 中的 fixture 只供自动化测试使用，不会被 `pnpm dev` 自动加载。主要页面的数据
来源如下：

| 页面 | 主要来源 |
| --- | --- |
| 总览 | `/api/v1/overview` 返回的配置摘要、网关状态、租约和 applied/desired 状态 |
| 网络设置 | `--config` 指定的 YAML，以及 Control Service 的本机网络发现结果 |
| 代理与规则源 | `--store` 下的 `sources.json`、凭据和来源快照 |
| 设备 | device policy、DHCP lease、邻居发现，以及 mihomo 当前连接 |
| 策略与节点 | applied mihomo API 中的策略组、节点健康和当前选择 |
| 连接 | 共享的设备归属快照、活跃连接、命中规则、出口链和速率 |
| 连通性与诊断 | applied runtime、日志、操作记录和 recovery 状态 |

使用 `examples/config.example.yaml` 与新的临时 store 时，看到的是示例网络配置加当前 Mac
的实际只读状态，而不是预置演示场景。该示例默认未配置 `device_policy.file`，临时 store
也没有来源或操作记录；mihomo 未运行时，节点、策略组、连接和流量自然为空或不可用。
需要对照当前真实订阅、设备和运行流量时，使用上述“已安装 Control Service + Vite”的
只读方式。

Control API 默认位置是 `http://127.0.0.1:61767`。端点描述、token、来源快照、操作记录
和局域网 DHCP 接管恢复状态位于：

```text
~/Library/Application Support/OpenSurge/
```

开发环境若确实需要执行网关动作，可用 root 身份运行 control service 并显式传入
`--direct-root`。普通开发和 UI 验证不要使用这个参数；未安装 helper 时 start/stop
会返回结构化错误，不会尝试提权或弹出不透明的 shell。

## 桌面与打包入口

从仓库根目录运行 `make desktop-build` 构建隔离 Preview，`make desktop-production-build` 构建正式身份的 App。环境要求和构建参数见 [桌面模块](../../../apps/desktop/README.md)。PKG 构建、校验与签名见 [构建与发布](releasing.md)。

## 验证

按改动运行 `make test`、`make web-test`、`make web-build`、`make desktop-test`；生产 bundle 使用 `make desktop-production-build`，打包契约使用 `./scripts/check-gui-packaging.sh`。

前端源码变化须刷新 `internal/webui/dist`。文档整理只检查内容、链接与差异，不由此宣称原生界面、PackageKit 或网络验收。原生交互见 [desktop smoke](validation/desktop-smoke.md)，网络行为见 [验证门槛](validation/test-gates.md)。旧 `menubar-test` 仅服务历史 Swift 维护。
