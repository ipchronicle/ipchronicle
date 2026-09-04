# IPChronicle v0.1.1 发布就绪报告

简体中文 | [English](RELEASE_READINESS.en.md)

状态：发布前验证中

本报告定义 `v0.1.1` 的范围、发布产物和验证门禁。最终候选中的
`release-manifest.json` 与 `checksums.txt` 会记录准确的源码修订和产物摘要。

## 版本身份

- 版本：`0.1.1`
- Tag：`v0.1.1`
- 渠道：`stable`
- 许可证：`AGPL-3.0-only`
- 源码：<https://github.com/ipchronicle/ipchronicle/tree/v0.1.1>
- Release：<https://github.com/ipchronicle/ipchronicle/releases/tag/v0.1.1>
- Center 镜像：`ghcr.io/ipchronicle/ipchronicle-center:v0.1.1`

## 发布范围

本版本交付以下产品边界：

- 单管理员认证、会话、TOTP 和服务器本地账户恢复；
- root Agent 注册、持久身份、30 秒轮询、临时 WebSocket 同步和原子更新；
- Linux AMD64/ARM64 公网出口发现、节点级代理、NAT 标记和地址变化历史；
- 手动、周期和新公网 IP 自动完整探测，以及结构化结果、原始结果和快照比较；
- Telegram 文字或图片、Webhook 和隔离 JavaScript 通知；
- 中英文管理界面；
- 独立的配置数据库和历史数据库；
- 普通反向代理与 Cloudflare Tunnel 两种 Docker Compose 部署示例。

## 验证门禁

普通 CI 对候选提交执行生成文件、格式、Lint、类型、普通单元测试、静态分析、
秘密扫描和生产 Web 构建。发布候选工作流始终构建并验证双架构 Agent、Center
镜像、SBOM、清单、校验和、版本和源码修订元数据。

候选工作流按照版本和自上一个稳定版以来的改动范围选择附加门禁：源码与竞态、
发行版生命周期、资源限制与真实探测、Compose、浏览器、失败恢复和可复现构建。
无法可靠分类时执行完整验证。被分类为不适用的门禁可以跳过；所有选中的门禁必须
成功，候选才可进入发布流程。

<!-- release-evidence:start -->
候选尚未完成验证。
<!-- release-evidence:end -->

## 发布产物

候选目录包含：

- Linux AMD64/ARM64 无 CGO Agent 二进制；
- Linux AMD64/ARM64 OCI Center 镜像；
- Agent 与 Center 的 CycloneDX SBOM；
- `compose.yaml`、`compose.cloudflare-tunnel.yaml` 和 `default.env.example`；
- `install-agent.sh`；
- 运维文档、许可证、第三方声明和构建元数据；
- 覆盖全部受控文件的 `release-manifest.json` 与 `checksums.txt`。

发布验证器会拒绝缺失、多余、超大、摘要不符或执行权限异常的文件。正式发布工作流
只接受同一源码提交上已经成功完成的候选产物。

## 部署与数据边界

- Center 支持 Linux + Docker Compose，TLS 由管理员维护的反向代理终止。
- Agent 必须以 root 运行，支持文档列出的 AMD64/ARM64 Linux 发行版和
  systemd/OpenRC。
- `v0.1.0` 没有投入需要保留数据的正式环境，当前没有持久数据兼容基线。
- `v0.1.1` 使用全新部署，不迁移开发版、候选版或 `v0.1.0` 的配置库、历史库和
  Agent 本地状态。
- Compose 把配置和历史保存到安装目录下的 `./data/config` 和
  `./data/history`。
- 产品没有内置备份或恢复功能。需要保留数据时，由服务器操作者一致备份两个数据
  目录及相关 Agent 状态。

## 发布条件

只有候选提交的普通 CI 和分类选中的全部发布门禁成功，且 annotated tag 指向同一
提交后，才运行正式发布工作流。该工作流重新验证候选、发布并检查双架构 GHCR
镜像和 GitHub Release；全部成功后将稳定版设为 Latest，并更新 GHCR `latest`。
