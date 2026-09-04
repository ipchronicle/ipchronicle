# IPChronicle v0.1.1

简体中文 | [English](RELEASE_NOTES.en.md)

IPChronicle 是一项个人自托管服务，用于发现受管 Linux 节点可到达的公网 IP、
执行完整 IP 质量探测，并持续记录有意义的结果变化。

这是首个准备投入正式使用的 IPChronicle 版本。`v0.1.0` 没有正式环境数据，
本版本采用全新部署，不提供从开发版、候选版或 `v0.1.0` 数据升级的迁移。

## 主要内容

- root Agent 直接运行有界、无 CGO 的 Go 版完整 IP 质量探测，支持 Linux
  AMD64/ARM64；单个提供商失败时只影响自身字段。
- 以公网 IP 作为探测对象。当前和历史地址在视觉上明确分离，报告始终关联实际
  产生报告的公网 IP。
- 发现直连与节点级代理出口，支持 IPv4/IPv6 和 NAT 标记，新公网 IP 默认启用，
  并可在已建立节点发现新地址时自动探测。
- 一次性手动目标选择与周期探测启用状态相互独立。计划任务使用可搜索的 IANA
  时区，并显示下一次执行时间。
- 提供监控概览，把系统状态归入设置，并让节点详情聚焦当前公网 IP、探测和历史。
- 支持广泛的通知路由、Telegram 群组和可选话题、未保存目标测试，以及所有事件
  可选文字或图片投递。已知探测值会显示为人类可读的简体中文或英文，原始 JSON
  和机器事件值保持不变。
- 支持可选的 Center 统一 ipapi API Key，移除未使用的 IPWHOIS 字段，并限制
  IPQS 瞬时失败重试。
- Center 与 Agent 时钟存在偏差时，Center 下发探测和 Agent 更新仍可恢复。因
  时间窗外任务报告而持续重试的 RC4 Agent，非 purge 重装本版本后可以恢复。
- 简化 Docker Compose 部署：配置与历史分别写入当前目录的 `./data/config`
  和 `./data/history`，并提供只需 Tunnel token 的 Cloudflare Tunnel 示例。
- 日常 CI 与发布验证按改动风险分级；稳定 Release 成功后才更新 GHCR `latest`。

## 部署

- Center：Linux + Docker Compose，提供 `linux/amd64` 和 `linux/arm64` 镜像。
- Agent：在文档列出的 Linux 发行版上以 root 服务运行，支持 systemd/OpenRC
  和 AMD64/ARM64。
- TLS：需要 HTTPS 时，由管理员维护的反向代理终止 TLS。

安装与运行方式见 [运维指南](OPERATOR_GUIDE.md)。

## 已知边界

- 只支持一位本地管理员，不包含多人、租户、角色或公开结果模式。
- 没有内置备份或恢复流程。需要保留数据时，应一致地备份两个 Center 数据目录
  和 Agent 状态。
- 第三方数据库和媒体服务可能改变响应或限流。未知值保持原样显示，不猜测也不
  静默映射。
- 允许使用 HTTP Center URL，但传输过程不受保护。
