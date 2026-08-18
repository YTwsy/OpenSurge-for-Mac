# 持久化网关规则

Web GUI 的“网关规则”页面用于补充当前 mihomo 订阅规则。它不会修改订阅快照，也不写回
主 gateway YAML；规则保存在 `runtime/gateway-rules.json`，因此刷新或替换订阅时仍会保留。

文件格式与 Clash Verge 的规则增强思路一致：

```json
{
  "schema_version": 1,
  "prepend": ["DOMAIN-SUFFIX,example.com,Proxy"],
  "append": ["DOMAIN-SUFFIX,internal.example,DIRECT"],
  "delete": ["DOMAIN-SUFFIX,unwanted.example,Proxy"]
}
```

- `prepend` 在订阅 `rules` 之前插入；
- `append` 在订阅 `rules` 之后、OpenSurge 默认规则之前插入；
- `delete` 按完整规则文本精确移除订阅中的规则，订阅刷新后仍会继续移除；
- 每一项必须是一条不含换行、Unicode 行分隔符或其他控制字符的完整规则；
- 规则最后的策略目标必须存在于当前 mihomo 配置，例如订阅中的代理组或 `DIRECT`。

OpenSurge 自己的网关保护、Mac 本机路由和设备专属规则仍位于自定义规则之前，避免自定义
规则破坏网关生命周期与设备身份隔离。保存时会把自定义规则合并进完整候选 mihomo 配置并
执行引擎校验；网关运行中保存后还需要重载，才能让当前 mihomo 进程加载新规则。

对应 API 为 `GET/PUT /api/v1/gateway-rules`，使用 `ETag` / `If-Match` 做乐观并发控制。
