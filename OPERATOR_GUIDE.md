# IPChronicle 运维指南

简体中文 | [English](OPERATOR_GUIDE.en.md)

本指南说明一位自托管管理员如何运行一套受支持的 IPChronicle。命令使用
`ipchronicle/ipchronicle` 官方 GitHub 仓库发布的产物。

## 支持环境

Center 支持在 Linux 上使用 Docker Engine 和 Docker Compose 运行。它不负责
终止 TLS 或配置反向代理。发布门禁在 512 MiB 内存限制下验证 Center。

root Agent 在以下 AMD64 和 ARM64 Linux 系统上受支持：

- Debian 12 和 13；
- Ubuntu 24.04 和 26.04；
- RHEL、Rocky Linux 和 AlmaLinux 8、9、10；
- CentOS Stream 9 和 10；
- 使用 OpenRC 的 Alpine Linux 3.23 和 3.24。

安装器会拒绝其他操作系统、发行版版本、架构和 init 系统。完整探测已在
64 MiB 内存下验证。内存低于 64 MiB 的 Agent 会继续观察地址，但默认暂停
完整探测，直到管理员启用低内存覆盖选项。

Center 需要通过出站 HTTPS 访问 GitHub 以发现版本。每个 Agent 需要通过出站
HTTP 或 HTTPS 访问 Center、已配置的公网 IP 发现服务、用于安装和更新的官方
GitHub Release，以及完整探测所使用的第三方数据库、流媒体、AI、DNSBL 和
邮件服务。网络代理只作用于明确引用它的发现路径，不是 Center 或 Agent 的
全局代理。

## 安装 Center

选择版本并创建空安装目录，然后下载部署产物。版本号不要包含开头的 `v`。

```sh
IPCHRONICLE_VERSION=0.1.0
mkdir ipchronicle
cd ipchronicle
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/compose.yaml"
curl --proto '=https' --tlsv1.2 -fL \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/default.env.example" \
  -o .env
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/checksums.txt"
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/release-manifest.json"
grep -E '  (default\.env\.example|compose\.yaml|release-manifest\.json)$' checksums.txt |
  sed 's/  default\.env\.example$/  .env/' | sha256sum --check
chmod 0600 .env
```

启动前检查 `.env`。只有 `config.db` 中尚无管理员账户时，启动凭据才会生效。
保持默认值时，首次登录为 `admin` / `admin`；界面会提示，但不会强制修改。

```sh
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
docker compose --env-file .env -f compose.yaml exec -T center \
  /usr/local/bin/ipchronicle-center healthcheck
```

用浏览器打开对外地址。在**账户**页面修改管理员用户名或密码、选择简体中文
或英文，并按需启用 TOTP 两步验证。修改 `.env` 中的启动凭据不会改变已有账户。

发布版 Compose 会创建两个独立卷：

- `ipchronicle_center-config` 保存 `config.db` 和 `master.key`；
- `ipchronicle_center-history` 保存 `history.db`。

不要把配置卷当作可随历史一起删除的数据。`master.key` 必须与对应的
`config.db` 一起保存；丢失它后无法恢复加密凭据。

## 环境变量

发布产物 `default.env.example` 提供以下运维设置：

| 变量                          | 默认值  | 用途                                        |
| ----------------------------- | ------- | ------------------------------------------- |
| `IPCHRONICLE_HTTP_PORT`       | `8080`  | Compose 发布到主机的端口                    |
| `IPCHRONICLE_ADMIN_USERNAME`  | `admin` | 首次启动的管理员用户名                      |
| `IPCHRONICLE_ADMIN_PASSWORD`  | `admin` | 首次启动的管理员密码                        |
| `IPCHRONICLE_TRUSTED_PROXIES` | 空      | 可提供转发请求头的代理来源 CIDR，以逗号分隔 |

`compose.yaml` 中固定的数据库路径和监听地址与两个卷相匹配。修改这些容器
内部值不属于受支持的发布部署方式。

## 反向代理与 TLS

建议在管理员维护的反向代理上使用 HTTPS，但产品不强制。只把 Center 实际
接收代理连接的来源 CIDR 写入 `IPCHRONICLE_TRUSTED_PROXIES`；该地址可能是
Docker bridge CIDR，而不是 `127.0.0.1`。外部地址在系统设置页面管理：自动
模式跟随当前浏览器请求，自定义值用于 Agent 安装命令和通知链接。

转发原始主机名、客户端地址和协议。`/api/v1/agent/sync/` 下必须支持 WebSocket
Upgrade；临时 WebSocket 同步不可用时，30 秒一次的 Agent HTTP 轮询仍是事实
来源。最小 Nginx 配置如下：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

IPChronicle 不申请证书、不把 HTTP 重定向到 HTTPS，也不修改代理。用户有意使用
HTTP 时仍可工作，但界面会提示。修改 `.env` 后重启 Center：

```sh
docker compose --env-file .env -f compose.yaml up -d
```

## 注册 Agent

打开**节点**。如果还没有自动注册密钥，先轮换生成一个，然后在每个受支持节点
上以 root 运行页面显示的命令。这条命令下载固定的官方安装器，由安装器根据部署
的 stable 或 RC 渠道选择最新 Agent，验证官方清单和校验和，安装下载依赖，注册
节点，并启动 systemd 或 OpenRC 服务。Center 不把 Agent 固定到自身版本。只有
明确需要某个版本时，管理员才直接给安装器传入 `--version VERSION`。

共享注册密钥只用于注册。每个 Agent 会取得节点专用凭据，并把它与本地队列一起
加密存放在仅 root 可读的 `/var/lib/ipchronicle-agent`。重复运行安装命令会保留
有效的本地身份。关闭自动注册只会阻止新节点，不会断开已注册 Agent。

注册不会执行完整探测。默认本地计划在 Agent 时区的午夜启用，首次是否探测由
管理员手动决定。

检查 Agent：

```sh
# systemd
systemctl status ipchronicle-agent
journalctl -u ipchronicle-agent -f

# OpenRC
rc-service ipchronicle-agent status
tail -f /var/log/ipchronicle-agent.log
```

两分钟内上报过的节点视为在线。配置通常在下一次 30 秒轮询时收敛。在节点页面
开启临时同步后，WebSocket 唤醒最多维持十分钟；它不会取代轮询，也不会创建
Agent 入站端口。

禁用节点后，Agent 收到变更便停止轮询工作和本地计划。吊销凭据会永久断开该
Agent。删除节点会移除 Center 管理的配置、隐藏发现路径和节点级状态，但不会
卸载主机软件。已经归入全局公网 IP 身份的报告和地址事件会保留。

## 公网 IP 发现与地址观察

在**设置 > 网络探测**中，为每个地址族配置 2 到 8 个不同主机的公网 IP 发现
服务。接受 HTTP 和 HTTPS URL；HTTP 会产生警告。只有配置的服务取得一致结果
时才确认地址，因此服务故障会明确显示，而不会被当成地址变化。

HTTP、HTTPS 和 SOCKS5 代理也在该页面集中配置。代理密码加密保存在
`config.db`，只会发送给引用该代理发现路径的 Agent，且不会再次显示。编辑时
密码留空会保留原值，除非明确选择**清除密码**。

打开节点的**公网 IP** 页面，管理该节点可到达的规范公网地址：

- 自动发现可用默认路由和稳定可路由源，并把它们作为隐藏路径；
- 同一公网 IP 即使由多个接口、来源、NAT 映射、代理或节点发现，在 Center 中
  也只出现一次；
- 新发现的公网 IP 默认启用完整探测，管理员可关闭；
- 显式代理发现路径把一个可复用代理及地址族绑定到一个节点，因为它无法从网络
  清单自动推断。

接口、路由、本地源地址、选择器和自动路径 ID 都是内部执行细节，不作为用户
对象显示。临时 IPv6 隐私地址不会产生独立持久路径。发现结果表明 NAT 时，
Center 会标记对应公网 IP。DNS 类检查使用节点解析器，可能走默认路由。

每个公网 IP 的完整探测开关默认开启。节点还有一个默认开启的设置：已建立的
发现路径把新公网 IP 加入节点当前确认集合时，为它执行完整探测。新路径的首次
观察只建立基线。默认发现间隔为十分钟。集合进入、退出、失败、恢复和节点级
队列缺口会保留；不记录没有变化的检查。

## 完整探测

在节点的**完整探测**页面执行探测或编辑计划。计划使用包含秒的六字段 Cron
表达式，并指定 `Asia/Shanghai` 等明确的 IANA 时区。注册密钥会记录管理员
浏览器时区，作为使用该密钥注册节点的默认值。停机期间错过的执行不会补跑；
节点已有探测运行时会跳过本次计划。IPChronicle 不额外限制用户设置的频率。

立即任务只为在线节点创建，两分钟后过期，占用节点唯一的任务槽，不存在任务
积压队列。页面会显示 Agent 是否收到任务，以及每个公网 IP 执行的进度或终态。

公网 IP 启用开关控制周期探测和新地址自动探测。节点级立即命令只为本次执行
选择目标，不改变这些开关。公网 IP 行上的立即操作会直接启动单 IP 任务，即使
该 IP 的周期探测已禁用也可以执行。

Agent 会针对每个目标公网 IP 运行内置 Go 探测，并校验有界 JSON 输出。HTTP、
HTTPS 和 SMTP 检查使用选定的源、接口或代理路径；DNS 解析和 DNSBL 查询使用
节点解析器。单个第三方提供商失败时，只把该提供商字段留空，不伪造数据，也不
导致无关检查失败。缺少的已知字段显示为缺失；已知字段变为不兼容类型时显示为
不可用，同时保留可检查的原始报告和格式状态。

Center 不可用时，每个公网 IP 的 Agent 最多保留 30 个待上报地址事件和 30 份
待上报完整结果；每条隐藏发现路径的地址事件分别计数。淘汰最早项目时会报告
明确的历史缺口。上传重试只重传已存数据，绝不重新执行探测。

## 历史、比较与保留策略

**历史**页面可按节点、公网 IP、时间、状态、触发方式、格式状态和变化状态
筛选完整报告和地址变化。报告可以与按时间排序的基线比较，也可以与同一公网 IP
的另一快照比较。加星快照不受保留清理影响。

在**设置 > 历史与存储**中选择一种策略：

- 永久保留；
- 按 1 至 36,500 天保留；
- 按 1 MiB 至 1 TiB 的逻辑大小保留。

策略变化时、手动请求时和每六小时会执行清理。当前状态、加星快照、活跃通知
投递和必要比较基线受保护。受保护数据可能使物理占用超过逻辑大小预算；页面会
分别报告逻辑数据、受保护数据、数据库、WAL 和共享内存占用。

**清除观察历史**会删除地址事件、探测运行、执行、快照和缺口，但保留管理员、
节点、公网 IP 设置及隐藏路径、代理、计划、通知配置和待处理任务状态。它还会
推进历史代次，使 Agent 丢弃旧代次的排队数据。重置后不会自动执行完整探测。

`v0.1.0` 是配置库、历史库和 Agent 本地状态的首个受支持兼容基线。后续同一
主版本必须通过有序前向迁移和 Agent 状态升级边界保留稳定版数据；无法读取的
稳定版数据必须明确失败，不能静默替换。`v0.1.0` 之前的开发版和 RC 数据仍不
属于受支持升级来源。

## 通知

打开**通知**，配置 Telegram、通用 Webhook 或隔离 JavaScript 发送器，再为
地址、探测、缺口或格式事件创建规则。启用规则前先使用**测试发送**。已保存
发送器的测试与真实事件走同一持久队列、超时、重试和终态路径。

Telegram 目标接受私聊或群组 ID，以及可选话题 ID。每个发送器可选择图片或
文字格式，所有支持的事件都使用所选格式。创建表单可以在保存前同步发送一次
测试。已知探测字段和值会转换为适合人类阅读的本地化文案；Webhook 载荷和
JavaScript 事件对象保留机器值。

只有配置目标和启用匹配规则后才会发送通知。发送器凭据加密保存在
`config.db`。载荷、JavaScript API、队列限制、重试和脱敏行为见
[通知说明](NOTIFICATIONS.md)。

## Agent 与 Center 更新

服务器操作者通过 Docker Compose 更新 Center。下载新版本的 `compose.yaml`
和 `default.env.example`，把新增环境变量与现有 `.env` 对照，然后只替换
`compose.yaml`：

```sh
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
```

Center 开始服务前会运行数据库迁移。不要让旧 Center 使用已由新版本迁移的
数据库；回滚 Center 必须恢复兼容的升级前卷备份。

在**设置 > 系统**中选择 stable 或 RC 版本发现。该选择同时控制 Center 与
Agent 的版本发现，但不会自动更新 Center。在**节点**中选择存在可用版本的在线
节点并请求 Agent 更新。Agent 会在原子替换前校验平台、版本、能力、清单、大小
和校验和。新 Agent 无法启动或回报健康时，独立 root supervisor 会恢复旧二进制
和状态检查点。更新和探测共享唯一立即任务槽。如果目标版本使用不同本地状态
schema，Agent 会在替换前拒绝原地更新。

## 卸载 Agent

以 root 使用固定的官方安装器：

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh" |
  sh -s -- --uninstall
```

该命令移除服务定义和已安装二进制，但保留 `/var/lib/ipchronicle-agent` 中的
节点身份与待上报数据。以后重新安装会复用该身份。

如需一次操作移除 Agent 及全部本地状态：

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh" |
  sh -s -- --uninstall --purge
```

Purge 会不可逆地丢弃节点凭据、已保留配置、加密的代理凭据、更新恢复状态、
任务身份和待上传结果。以后安装会创建新节点。需要另外在 Center 中移除或吊销
旧节点；两种卸载模式都不会删除 Center 配置或历史。

## 本地管理员恢复

这些命令需要服务器级 Compose 权限，会吊销所有现有管理员会话，且不会暴露
远程恢复 API。

```sh
read -r -s IPCHRONICLE_RECOVERY_PASSWORD
printf '%s\n' "$IPCHRONICLE_RECOVERY_PASSWORD" |
  docker compose --env-file .env -f compose.yaml exec -T center \
    /usr/local/bin/ipchronicle-center admin reset-password --password-stdin
unset IPCHRONICLE_RECOVERY_PASSWORD

docker compose --env-file .env -f compose.yaml exec -T center \
  /usr/local/bin/ipchronicle-center admin disable-totp
```

## 备份与灾难恢复边界

首版没有内置备份或恢复命令。使用服务器操作者控制的卷或文件系统工具。复制
数据库前停止 Center，或使用理解 SQLite 的快照机制；只复制正在运行的数据库
文件而不包含 WAL 状态不是有效备份。

配置卷和历史卷应作为匹配的一组备份。配置卷用于恢复账户、Agent 身份、代理和
通知秘密、计划以及历史代次。历史卷可以通过界面独立重置，但 Agent 存在排队
数据时，直接删除它不能替代协调完成的**清除观察历史**操作。

需要在主机恢复后保留 Agent 身份和离线队列时，也要保存
`/var/lib/ipchronicle-agent`。丢失该目录后必须重新注册，并会生成新节点身份。

Center 启动失败时，先检查日志再改动数据：

```sh
docker compose --env-file .env -f compose.yaml ps
docker compose --env-file .env -f compose.yaml logs --tail=200 center
```

IPChronicle 不会静默重建缺失的 master key、替换损坏数据库，或在迁移失败后
报告成功。应恢复兼容备份，或明确决定只重置历史；不要把删除
`ipchronicle_center-config` 当成历史清理。
