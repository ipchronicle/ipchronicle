# 通知

简体中文 | [English](NOTIFICATIONS.en.md)

IPChronicle 会先持久记录地址、完整探测、历史缺口和上游格式事件，再计算通知
规则。每条规则选择一种事件、一个发送器，以及可选的已知字段、节点和网络出口
筛选条件。规则在处理持久事件时计算，而不是在事件首次观察时计算。

多条规则选择同一事件和发送器时，IPChronicle 只创建一次投递，并记录全部匹配
规则。投递和事件 ID 稳定，因此重启 Center 不会主动制造重复工作。通知事件和
投递历史保存在 `history.db`，遵循已配置的历史保留策略；活跃投递不受清理影响。

## 发送器

在管理界面的**通知**页面创建、编辑、测试、禁用或删除发送器。测试投递与事件
投递使用同一持久队列、worker、超时和重试路径，并显示在**投递历史**中。

### Telegram

填写 Bot API Token、目标私聊或群组 ID，以及论坛群组可选的话题 ID。Bot 必须
已经具备向目标和话题发送消息的权限。每个发送器选择一种格式：

- **图片**调用 Telegram 官方 `sendPhoto` API，发送有大小限制的黑色状态卡片，
  并在说明文字中附带详情链接；
- **文字**调用 Telegram 官方 `sendMessage` API，发送有大小限制的 HTML 消息，
  并关闭链接预览。

所选格式适用于所有支持事件。探测变化使用本地化的人类可读字段名称和值，包括
数据库分类、风险等级及因子、国家代码、媒体状态和邮件连通性。未知上游值保持
原样可见；用于机器处理的原始事件信封不会为展示而重写。

Token 加密保存在 `config.db`，API 和 Web 界面都不会返回。编辑时 Token 留空会
保留原值。创建表单可以用尚未保存的值同步测试一次；它只验证 Telegram 是否
接受消息，不会保存发送器、投递或规则。已保存发送器的测试走持久队列，并出现
在投递历史中。

### Webhook

填写 HTTP 或 HTTPS URL，并按需提供最多 32 个 `Name: value` 格式的请求头。
IPChronicle 使用 HTTP `POST` 发送版本化 JSON 事件信封，并设置以下基础请求头：

```text
Content-Type: application/json
User-Agent: IPChronicle-Notification/1
```

配置请求头可以替换这些值，但不能设置 `Host`、`Content-Length`、`Connection`
或 `Transfer-Encoding`。不接受包含用户信息或 fragment 的 URL。重定向会作为
失败返回，不会跟随。

请求头值加密保存在 `config.db`，只返回已配置的请求头名称。编辑 Webhook 时会
保留全部请求头值，除非启用**替换已配置请求头**。

### JavaScript

每次投递都会在一个新的隔离 goja worker 进程中运行已配置的 JavaScript。脚本
会收到一个全局对象：

```javascript
ipchronicle.apiVersion; // 1
ipchronicle.event; // 解析后的版本化事件信封
ipchronicle.title; // 本地化纯文本标题
ipchronicle.body; // 本地化纯文本正文
ipchronicle.http.request({
  method: "POST", // 可选，默认为 GET
  url: "https://example.invalid/notify",
  headers: { "Content-Type": "application/json" }, // 可选
  body: JSON.stringify(ipchronicle.event), // 可选
});
```

`ipchronicle.http.request()` 是同步调用，返回：

```javascript
{
  status: 204,
  headers: { "Header-Name": ["value"] },
  body: "",
}
```

未捕获异常会使本次尝试失败。非 2xx 响应不会自动导致脚本失败；如果目标要求
特定结果，应检查 `status` 并主动抛出异常。

worker 不提供 Node.js 或 Deno 运行时、模块加载、`require`、`fetch`、DOM、文件
系统、进程、环境变量、定时器或裸 socket API。唯一网络能力是上面的同步
HTTP/HTTPS 调用。它不使用 Center 进程配置的 HTTP 代理，也不跟随重定向。

每次调用受以下边界限制：

- 总墙钟时间 30 秒；
- 最多 10 个 HTTP 请求，每个请求不超过剩余时间且最多 10 秒；
- 每个请求正文和响应正文不超过 1 MiB；
- 最多 32 个请求头，每个名称/值对不超过 8 KiB；
- 源码不超过 256 KiB、事件输入不超过 1 MiB、worker 输出不超过 16 KiB；
- Linux 下 worker data segment 限制为 128 MiB。

这些边界限制意外资源耗尽，但不是面向互不信任管理员的策略沙箱。唯一的服务器
操作者拥有脚本，可以有意把事件数据或脚本内秘密发送到任意可达 HTTP/HTTPS
端点。

## 队列、重试与失败行为

IPChronicle 使用四个 Telegram/Webhook 共享 worker 和一个全局 JavaScript
worker。每个发送器最多存在 1,024 个活跃投递；继续匹配的事件会保留为终态
`queue-full` 失败，而不会创建无界工作。

每次投递最多尝试四次。第一次失败后等待 10 秒，第二次后等待一分钟，第三次后
等待五分钟。超时、连接失败、读取响应失败、HTTP `429`、HTTP `5xx`、Center
取消和 JavaScript worker 超时可以重试。其他 HTTP `4xx`、无效配置或请求、
超大响应和脚本错误为终态失败。

投递页面显示 `pending`、`running`、`retrying`、`succeeded`、`failed` 状态、
尝试次数和有界错误码。目标响应正文、异常消息、Telegram Token 和 Webhook
请求头值不会存入投递错误，也不会返回浏览器。

禁用发送器后，排队工作不会发送。规则仍引用发送器或发送器仍有活跃投递时不能
删除。删除后，已有终态投递历史仍保留发送器名称和类型。

## 事件类型

规则可以选择以下事件：

- 完整探测已知字段变化；
- 公网地址变化、检查失败和恢复；
- 完整探测失败和恢复；
- 地址历史和探测历史缺口；
- 上游格式不匹配、不匹配内容变化和格式恢复。

已知字段筛选只适用于完整探测字段变化规则。节点筛选可以和该节点的一个持久
网络出口组合使用。
