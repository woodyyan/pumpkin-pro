# 本地镜像发布脚本需求

## 背景

当前仓库的 GitHub Actions 同时承担镜像构建、推送 TCR 与服务器部署，构建链路较慢。项目已确定将 Phase 2 调整为“本地构建 + 本地 push TCR，GitHub Actions 后续只负责部署指定 tag”。

## 本阶段目标

- 提供一个统一的本地 shell 入口 `ops/local/release.sh`
- 支持按服务选择执行：`backend`、`frontend`、`quant`
- 支持 `--build-only` 与 `--push`
- 支持统一 tag 规则：`release-时间-shortsha`
- 输出 manifest，供后续部署和对账使用
- 允许先独立验证本地构建与推送 TCR

## 已确认约束

1. 支持独立构建的服务固定为：`backend` / `frontend` / `quant`
2. TCR 仓库命名规则：每个服务一个 repo
3. tag 规则：`release-时间-shortsha`
4. manifest 需要明确输出字段
5. Phase 2 暂不实现 GitHub Actions 纯部署改造
6. Phase 2 暂不实现回滚、远端校验、digest 闭环
