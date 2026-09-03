# IPChronicle v0.1.0 发布就绪报告

简体中文 | [English](RELEASE_READINESS.en.md)

状态：已于 2026-09-03 发布

本报告把公开版本与产品范围、自动验证、发布产物、运维文档和发布门禁对应起来。
`release-manifest.json` 与 `checksums.txt` 中的源码修订和摘要标识特定构建。

## 版本身份

- 版本：`0.1.0`
- Tag：`v0.1.0`
- 渠道：`stable`
- 许可证：`AGPL-3.0-only`
- 源码提交：`70174ebd26c2729f056a7e83462d1678b7722fd0`
- 源码：<https://github.com/ipchronicle/ipchronicle/tree/v0.1.0>
- Release：<https://github.com/ipchronicle/ipchronicle/releases/tag/v0.1.0>
- Center 镜像：`ghcr.io/ipchronicle/ipchronicle-center:v0.1.0`

## 产品范围证据

| 能力                                                           | 实现证据                                                                                                            | 确定性验证                                                   |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| 单管理员认证、会话、TOTP 和本地恢复                            | `internal/center/admin`、`cmd/ipchronicle-center`                                                                   | 包测试、Compose smoke、浏览器测试、迁移与恢复失败门禁        |
| Agent 注册、持久身份、30 秒轮询、配置收敛和临时同步            | `internal/agent`、`internal/center/nodes`、`internal/center/syncws`                                                 | 包与竞态测试、Compose smoke、浏览器测试、发行版生命周期测试  |
| Linux 接口、地址、路由、出口、代理、NAT 和临时 IPv6 处理       | `internal/agent/network`、`internal/agent/observation`、`internal/center/nodes`                                     | 清单、选择器、代理、观察、断网、重启和队列测试               |
| 手动、计划和新公网地址完整探测，单立即任务槽                   | `internal/agent/probe`、`internal/schedule`、`internal/center/nodes`                                                | 调度、原生执行、结果发布、重试、资源和真实完整探测测试       |
| 已知字段解释、原始结果、本地化展示、格式漂移、比较、加星和保留 | `internal/probefields`、`internal/center/history`、`internal/center/nodes`、`web/src/pages/probe-snapshot-page.tsx` | 解释、展示、比较、保留、重置、容量、API 和浏览器测试         |
| Telegram 文字/图片、Webhook 和隔离 JavaScript 通知             | `internal/center/notifications`、`cmd/ipchronicle-center`                                                           | 发送器、渲染、队列、重试、隔离、脱敏、溢出、API 和浏览器测试 |
| Agent 更新发现、校验、原子替换、健康提交和回滚                 | `internal/agent/update`、`internal/center/updates`                                                                  | 更新管理器、supervisor、回滚、发行版生命周期和浏览器测试     |
| 双语管理界面                                                   | `web/src`、`web/src/locales`                                                                                        | 语言资源一致性、组件、生产构建和桌面/移动 Chromium 测试      |
| 配置与历史独立所有权                                           | `internal/center/database/migrations`、独立 sqlc 包                                                                 | 迁移、损坏、重置、保留、Compose 和失败门禁测试               |

## 必需验证门禁

所有门禁均为失败关闭；缺失、取消、跳过或结论不明都不算通过。

| 门禁                                                                           | 命令或工作流证据                                               | v0.1.0 结果                                                                 |
| ------------------------------------------------------------------------------ | -------------------------------------------------------------- | --------------------------------------------------------------------------- |
| 本地版本、源码、生成文件、格式、Lint、类型、普通测试和静态分析                 | `make preflight`                                               | 通过                                                                        |
| 生成绑定、普通/竞态测试、原生 Center、无 CGO Agent、生产 Web 构建              | `make check`；GitHub Actions `CI / check`                      | [通过](https://github.com/ipchronicle/ipchronicle/actions/runs/33723135472) |
| 已提交源码秘密扫描                                                             | `make secret-scan`；GitHub Actions `CI / check`                | 通过，无泄漏                                                                |
| 生产 Center 镜像与 Compose 边界                                                | `make compose-smoke`；`CI / compose`                           | 通过                                                                        |
| 中英文桌面/移动流程                                                            | `make browser-test`；`CI / browser`                            | 通过                                                                        |
| AMD64/ARM64 Center 镜像元数据                                                  | `CI / image (linux/amd64)`、`CI / image (linux/arm64)`         | 通过                                                                        |
| 候选产物、清单、校验和、SBOM 和产物契约                                        | `Release candidate artifact / candidate`                       | [通过](https://github.com/ipchronicle/ipchronicle/actions/runs/33724520372) |
| 安装、重装、卸载、迁移、历史重置、断网、重启、选择器不可用、更新回滚和队列溢出 | `Release candidate artifact / candidate`                       | 通过                                                                        |
| 支持的发行版与 init 生命周期                                                   | 17 个发行版 x AMD64/ARM64                                      | 34/34 通过                                                                  |
| 原生资源限制和真实完整探测                                                     | AMD64/ARM64 `resources`                                        | 64 MiB Agent、512 MiB Center 均通过                                         |
| 70 节点、420 出口容量                                                          | `internal/center/nodes/release_capacity_test.go`；`make check` | 通过                                                                        |
| 可复现构建                                                                     | 从清单提交执行两次干净构建并逐文件比较                         | 完全一致                                                                    |
| 源码卫生                                                                       | ShellCheck、actionlint、`git diff --check`、干净工作区         | 通过                                                                        |
| 正式发布与匿名产物验证                                                         | `Publish release`                                              | [通过](https://github.com/ipchronicle/ipchronicle/actions/runs/33726934524) |

## 验证结果

最终候选提交 `70174ebd26c2729f056a7e83462d1678b7722fd0` 于 2026-09-03
完成普通 CI、候选发布矩阵、可复现构建和发布工作流。候选矩阵包含 39 个任务，
全部通过且无失败。正式 Release 为非草稿、非 prerelease，并作为仓库 Latest
Release 发布；GHCR manifest 同时包含 `linux/amd64` 和 `linux/arm64`。

正式镜像还完成了本地 Compose 与真实测试机 Agent 验收。测试机 Agent 版本为
`0.1.0`，systemd 正常运行；最终 IPv4/IPv6 完整探测 2/2 成功，两份快照格式
问题均为 0。

## 发布产物契约

发布目录包含以下面向操作者或可由机器验证的产物：

- Linux AMD64/ARM64 无 CGO Agent 二进制；
- Linux AMD64/ARM64 OCI Center 镜像；
- 两种 Agent 与两种 Center 镜像的 CycloneDX SBOM；
- `compose.yaml` 和 `default.env.example`；
- `install-agent.sh`；
- `README.md`、`OPERATOR_GUIDE.md`、`NOTIFICATIONS.md` 和本报告；
- `LICENSE`、`THIRD_PARTY_NOTICES.md` 和 `build-metadata.json`；
- 覆盖受控产物集合的 `release-manifest.json`；
- 覆盖所有受控产物与清单的 `checksums.txt`。

发布验证器会拒绝缺失、多余、非普通文件、超大、被篡改或执行权限异常的文件。
清单按需记录源码修订、渠道、能力、大小、SHA-256、操作系统和架构。

`RELEASE_NOTES.md` 是 GitHub Release 说明的来源。候选构建器会拒绝版本不一致
的公开文档和工作流默认值。

## 运维流程覆盖

`OPERATOR_GUIDE.md` 是独立运维入口，覆盖 Center 安装与更新、环境变量、反向
代理、Agent 注册与服务检查、出口和代理配置、地址观察、完整探测、历史与比较、
保留与重置、通知、Agent 更新与回滚、卸载、本地账户恢复，以及备份/灾难恢复
边界。`NOTIFICATIONS.md` 定义发送器配置、JavaScript API、队列限制、重试、
失败隔离和脱敏行为。

## 限制与边界

- 产品只服务一位个人自托管管理员，不包含多人、租户、角色或公开结果功能。
- Center 只支持 Linux + Docker Compose，不终止 TLS，也不配置管理员的反向代理。
- Agent 必须以 root 运行，只支持文档列出的 AMD64/ARM64 Linux 发行版矩阵与
  systemd/OpenRC。
- 首版没有内置备份或恢复命令；管理员负责一致备份卷和 Agent 状态。
- 持久数据兼容从干净的 `v0.1.0` 部署开始；更早的开发版和 RC `config.db`、
  `history.db`、Agent 状态不是受支持升级来源。
- 内置 Go 探测基于 `THIRD_PARTY_NOTICES.md` 记录修订对应的 AGPL IPQuality
  行为；数据库、媒体、AI、DNSBL 和邮件检查仍依赖 IPChronicle 无法控制的
  第三方服务可用性与格式。
- HTTP、HTTPS 和 SMTP 检查使用所选出口路径；DNS/DNSBL 查询使用节点解析器，
  不通过出口代理。界面会报告提供商数据不可用和格式变化，不承诺未实现的路由。

## 发布结论

所有门禁均已通过。明确的发布操作从清单提交构建并重新验证正式产物，发布双架构
Center 镜像与 GitHub Release，完成匿名下载验证后，把 `v0.1.0` 设为 stable
渠道的 Latest Release。该过程未隐式发布任何候选版本。
