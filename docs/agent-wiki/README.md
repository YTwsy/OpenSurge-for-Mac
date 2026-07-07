# OpenSurge Agent Wiki

这个目录是 LLM wiki 风格知识层的手工种子。它不是面向终端用户的产品文档，
而是为了让未来 agent 从稳定的项目上下文开始工作，不必每次都从全仓库重新
拼出同一套心智模型。

当前阶段刻意保持很小：把稳定项目知识放在代码旁边，同时让目录形状兼容未来
接入 `llm-wiki-compiler` 的刷新、审查与上下文包工作流。

## 目录结构

```text
docs/agent-wiki/
  README.md
  sources/
    project-brief.md
    decisions/
      mihomo-profile-overlay.md
      tun-mainline.md
    validation/
      lab-gates.md
      real-device-smoke.md
  wiki/
    index.md
    concepts/
      gateway-lifecycle.md
      macos-tun-transparent-proxy.md
      mihomo-profile-overlay.md
      validation-gates.md
```

`sources/` 保存来源材料：稳定事实、产品方向、决策和验证契约。

`wiki/` 保存已整理的上下文页面：短小、互相链接，并且适合 agent 在相关任务
开始前优先阅读。

未来 compiler 的本地状态应放在 `docs/agent-wiki/.llmwiki/`，除非项目明确
改变决定，否则不要提交进 git。

## 更新规则

当某个变化会影响未来 agent 的工程判断时，更新或新增 wiki 知识：

- 网关生命周期的不变量发生变化；
- DHCP、DNS、mihomo、pf、sysctl 或 runtime state 的职责边界移动；
- 透明代理策略变化；
- lab gate 的含义或验收信号变化；
- 真实故障沉淀出可复用的排查规则；
- 产品定位变化，并会影响实现取舍。

不要写入一次性日志、临时命令输出、未经验证的猜测或普通 TODO。

## 刷新流程

当前阶段手工维护：产生知识变化的代码或文档 PR，应同时更新 `sources/` 和
`wiki/`。

未来接入 `llm-wiki-compiler` 时，可以把这个目录作为项目根：`sources/`
继续作为来源材料，`wiki/` 继续作为审查后的编译输出，`.llmwiki/` 存放本地
编译状态。
