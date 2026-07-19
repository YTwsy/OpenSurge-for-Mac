# 为 OpenSurge for Mac 做贡献

感谢你关注 OpenSurge for Mac。

## 开始之前

- 请先阅读 `README.md`，并浏览 `docs/`。
- 涉及网关、透明代理、配置或验证时，请继续阅读 `AGENTS.md` 和
  `docs/agent-wiki/wiki/index.md`。
- 欢迎大胆、未完成，甚至有点异想天开的想法；请简单说明它想解决什么问题。

## 提交改动

- 尽量让每次改动保持小而清楚。
- 如果行为、约束或验证方式发生变化，请同步更新相关 `docs/`。
- 至少运行 `make test`。网络或 TUN 改动请根据
  `docs/agent-wiki/wiki/concepts/validation-gates.md`，说明实际运行了哪些验证、
  哪些尚未验证。
- 不要提交订阅、节点、密钥、个人网络信息或未经脱敏的日志。

不确定想法是否可行也没关系，欢迎直接创建 Issue 或 Draft PR 讨论。
