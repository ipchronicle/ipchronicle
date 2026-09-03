# IPChronicle

简体中文 | [English](README.en.md)

[![CI](https://github.com/ipchronicle/ipchronicle/actions/workflows/ci.yml/badge.svg)](https://github.com/ipchronicle/ipchronicle/actions/workflows/ci.yml)
[![最新版本](https://img.shields.io/github/v/release/ipchronicle/ipchronicle?display_name=tag&sort=semver&label=%E6%9C%80%E6%96%B0%E7%89%88%E6%9C%AC)](https://github.com/ipchronicle/ipchronicle/releases/latest)
[![许可证：AGPL-3.0-only](https://img.shields.io/badge/%E8%AE%B8%E5%8F%AF%E8%AF%81-AGPL--3.0--only-0f766e)](LICENSE)

IPChronicle 是面向个人自托管用户的公网 IP 与 IP 质量历史系统。它通过用户
管理的 Linux 节点发现稳定的公网 IPv4 和 IPv6 地址，记录已确认的地址变化，
按需或按本地计划执行内置的完整 IP 质量探测，比较保留的报告，并发送变化通知。

本仓库包含完整产品：Go Center、root Linux Agent、React Web 界面、OpenAPI
契约、Docker Compose 部署、测试和发布工具。Center 在 Linux 上通过 Docker
Compose 运行；Agent 只主动连接 Center，不需要开放入站管理端口。

IPChronicle 采用 `AGPL-3.0-only` 许可证。产品范围和架构决策维护在
[IPChronicle workspace](https://github.com/ipchronicle/workspace/tree/main/docs) 中。

## 核心能力

- 通过直连和节点级代理路径发现不同的公网 IPv4/IPv6 出口，同时避免把网卡级
  拓扑暴露成用户需要管理的产品对象。
- 支持手动探测、本地周期计划，以及已建立节点发现新公网地址后的自动探测。
- 保留地址变化和报告历史，比较快照，并下载或复制报告图片。
- 通过 Telegram、Webhook 或 JavaScript 发送可配置的文字或图片通知。
- 管理界面支持简体中文和英文，可选启用 TOTP 两步验证。
- AMD64/ARM64 Linux Agent 支持 systemd 和 Alpine/OpenRC。

## 快速开始

从[最新稳定版](https://github.com/ipchronicle/ipchronicle/releases/latest)获取
受控发布产物，并按照[在线部署指南](https://ipchronicle.github.io/guide/deploy-center)
使用 Docker Compose 部署 Center。登录后，节点页面会生成一条命令，用于注册并
启动每个 root Agent。对应版本的 Release 同时提供冻结的离线运维指南。

Center 不终止 TLS，也不管理反向代理。建议使用 HTTPS，但证书和反向代理仍由
自托管管理员负责。

## 文档

- [在线文档](https://ipchronicle.github.io/) · [English](https://ipchronicle.github.io/en/)
- [随当前源码冻结的运维指南](OPERATOR_GUIDE.md) · [English](OPERATOR_GUIDE.en.md)
- [随当前源码冻结的通知说明](NOTIFICATIONS.md) · [English](NOTIFICATIONS.en.md)
- [版本说明](RELEASE_NOTES.md) · [English](RELEASE_NOTES.en.md)
- [发布就绪报告](RELEASE_READINESS.md) · [English](RELEASE_READINESS.en.md)
- [产品与架构决策](https://github.com/ipchronicle/workspace/tree/main/docs)

在线文档是最新稳定版安装、反向代理、Agent 注册、公网 IP 发现、探测、历史、
更新、恢复和卸载的入口。每个发布版本都会附带对应版本的冻结文档、Compose
文件、Agent 安装器、校验和、清单、SBOM、构建元数据和发布就绪报告。

## 开发

仓库级开发需要 Docker 与 Docker Compose、GNU Make、`curl` 和 `jq`。直接开发
前端使用 Node.js 24.19.0 和 npm 11.17.0；直接开发 Go 使用 Go 1.26.5。标准
检查使用固定版本的容器：

```sh
make generate
make ci
make preflight
```

直接构建 Center 时必须设置 `GOEXPERIMENT=nogreenteagc`；Make 和 Docker 构建
会自动应用该设置，以维持 Go 1.26 下 JavaScript worker 的内存限制边界。

- `make generate` 重新生成 Go/TypeScript OpenAPI 绑定，以及相互独立的
  `config.db`、`history.db` sqlc 包。
- `make ci` 执行日常提交使用的生成文件、格式、Lint、类型、单元测试、静态分析
  和生产前端构建检查。
- `make preflight` 是推送前的轻量仓库完整性检查。
- `make check`、Compose、浏览器、双架构和发布矩阵等扩展检查由 GitHub Actions
  定时执行，或按改动路径和发布风险执行。本地目标主要用于定位对应的 CI 失败。

仓库所有权和工程规则见 [AGENTS.md](AGENTS.md)。

## 贡献与安全

提交 Pull Request 前请阅读组织级[贡献指南](https://github.com/ipchronicle/.github/blob/main/CONTRIBUTING.md)。
Bug 和功能建议请使用结构化 [Issue](https://github.com/ipchronicle/ipchronicle/issues/new/choose)。
疑似安全漏洞请按照[安全政策](https://github.com/ipchronicle/.github/blob/main/SECURITY.md)
进行私密报告。
