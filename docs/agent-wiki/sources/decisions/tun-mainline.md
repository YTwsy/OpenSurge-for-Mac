---
title: TUN is the macOS transparent proxy mainline
kind: decision
status: accepted
---

# TUN 是 macOS 透明代理主线

TUN 是 OpenSurge for Mac 在 macOS 上受支持的透明代理路径。

项目曾评估 `mihomo.redir_port` 加 PF TCP redirection 的路线，但这条路线现在
明确 inactive。当前 Darwin mihomo build 在运行时报告 redir 不受支持，因此
仓库把 PF TCP redirection 视为已退役的透明代理实现路径。

## 影响

- `transparent.mode: "tun"` 是受支持的透明代理模式。
- `mihomo.redir_port` 必须保持 `0`。
- `pf.redirect_tcp_to` 必须保持 `0`。
- `internal/mihomo/config.go` 不应重新启用 `redir-port` 路径。
- `internal/pf/template.go` 不应重新输出 `rdr pass` TCP redirect 规则。
- 当用户尝试启用已退役旋钮时，配置验证应指向
  `transparent.mode: "tun"`。
- 透明代理相关变更应通过 `make lab-test-tun` 验证。

## 重新打开条件

只有当 macOS-compatible mihomo redir 支持真实存在、可测试，并且相对 TUN 有
明确产品理由时，才重新打开这个决策。重新打开需要同时改代码、文档和 lab
覆盖。
