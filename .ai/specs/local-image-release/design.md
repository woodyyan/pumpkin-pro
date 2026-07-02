# 本地镜像发布脚本设计

## 方案概述

Phase 2 采用“本地制品生成”方案：开发者在本机执行 `ops/local/release.sh`，按服务构建 Docker 镜像，并可选择推送到 TCR。脚本生成 `.release/manifests/<tag>.json` 记录本次发布结果。

## 模块拆分

### 1. `ops/local/release.config.sh`

负责维护稳定配置：

- 支持的服务清单
- 服务 -> repo 名映射
- 服务 -> build context 映射
- 服务 -> Dockerfile 映射

该文件避免把服务映射写死在主脚本逻辑里，后续新增服务时只需改配置文件。

### 2. `ops/local/release.sh`

负责运行时流程：

- 解析参数
- 校验 tag、服务名、并发参数
- 按服务构建镜像
- 在 `--push` 模式下登录 TCR 并推送镜像
- 输出 manifest

### 3. `ops/local/release.test.sh`

提供轻量 dry-run 测试，用于验证：

- 参数解析成功
- 多服务可执行
- manifest 能生成
- manifest 中包含目标 repo 信息

## manifest 结构

- `release_id`
- `tag`
- `mode`
- `created_at`
- `git.branch`
- `git.commit`
- `git.short_sha`
- `image_registry`
- `image_namespace`
- `image_platform`
- `requested_services`
- `services[]`
  - `name`
  - `repo`
  - `tag`
  - `image_ref`
  - `latest_ref`
  - `pushed_latest`
  - `dockerfile`
  - `context`
  - `build_status`
  - `push_status`
  - `image_id`

## 当前阶段边界

- 仍按串行顺序构建，不启用真正并行
- 默认不推 `latest`，需显式 `--push-latest`
- 不做远端 tag 存在校验
- 不做 digest 级别发布闭环
- 不改 GitHub Actions deploy 流程
