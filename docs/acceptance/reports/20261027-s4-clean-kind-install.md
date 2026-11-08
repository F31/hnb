# S4 干净环境独立安装验收（kind / Minimal / existing 模式）

- run_id: `35e92df2139d40765314d4e1cb1027dc`（install run，status: succeeded）
- 日期: 2026-10-27
- 环境: kind 集群 `hnb-s4-clean`（K8s v1.36.1，单 control-plane，StorageClass `standard`）
- 制品库: 本地 Harbor `harbor.hnb.local`（HTTPS，CA 已分发至 kind 节点 containerd 信任链）
- result: **passed（带已知缺口，见末节）**

## 制品与锁

| 物料 | 身份 |
|---|---|
| ReleaseLock | `release.lock.yaml`，status=candidate，20 个镜像（linux/amd64 manifestDigest），迁移链 platform/market/artifact 各 107 条 checksum，Chart + hnbctl digest 固定 |
| Chart 包 | `hnb-0.1.0.tgz`（锁内 digest 校验，`render` 对不匹配的包 fail-closed） |
| hnbctl | `artifacts/hnbctl`（`-trimpath` 构建） |
| SBOM | 16 个 CycloneDX（`go version -m` 提取）+ index.yaml |

## 渲染校验

- 19 个工作负载，9 个 Service，**0 个 Secret**（全部经 existingSecret 引用）
- 全部镜像为 `harbor.hnb.local/hnb/*@sha256:` 且均在锁内
- `helm lint` 无 bindings 时 fail-closed（设计使然）

## 安装与运行

- `hnbctl install --execute`：planning / platform-release / platform-verified / database-migration 全绿，run succeeded
- 迁移：platform 107 条（手工预验一次 + 正式 run skipped 幂等），market/artifact 由 run applied
- 部署后 **19/19 Pod Ready**（含 postgresql-0、nats-0）
- 幂等：`hnbctl resume --run-id 35e92df…` 返回 `run already succeeded; resume is not applicable`（终态 run 拒绝二次进入，无副作用）

## 冒烟证据

- Portal `GET /` → 200（svc/portal port-forward）
- 登录 `POST /api/v1/auth/login`（admin / hnb-admin Secret）→ 200，签发 ES256 access token（kid `s4-1`）
- `GET /api/v1/resources/clusters` → 200，返回 host-cluster + 2 个 GSLB demo 集群
- cluster-agent 主机集群 onboarding：DB 插入 host-cluster target（`00000000-0000-4000-8000-00000000000a`）→ 真实 API `POST /api/v1/resources/clusters/{id}/agent-onboarding` 铸币 → Secret `hnb-agent-token` → agent 经 `ws://tunnel-server:9443/tunnel` 注册成功，心跳正常（tunnel-server 日志 `agent … registered (total: 1)`）

## 本轮修复的发布阻断缺陷（代码已改，未提交）

1. `tunnel-server` / `kubernetes-provider` Chart 缺 Service 模板 → 隧道与 provider 调用链不可达。已补 `templates/service.yaml`，并修正 tunnel-server containerPort（9443，原误标 8080）。
2. `operation-worker` runtimeProviders JSON 曾带残留花括号 → 解析失败 CrashLoop（bindings 侧修正）。
3. `runtime-target-lifecycle-provider` bindings audience 误配 `hnb-platform-api` → 二进制 fail-closed 拒绝启动。应为 `hnb-runtime-target-lifecycle-provider`（provider.Audience）。
4. `cluster-agent` Minimal 未配 onboarding → 二进制要求 AGENT_TOKEN_FILE。已按 Chart 注释设计走 onboarding.existingTokenSecret 显式绑定。
5. `cmd/hnbctl/internal/toolkit/render.go`：helm 失败时吞掉 stderr，现回报真实模板错误（可观测性修复）。
6. apiserver `audiences` 需包含 `hnb-apiserver`（bindings 修正）。

## 已知缺口（S4 5.10 勾选前必须闭环）

1. **DSN 主机解析依赖**：hnbctl 迁移阶段要求 DSN 主机名在操作机可解析。实验室用 `/etc/hosts` + `kubectl port-forward svc/postgresql 15432:5432` 绕过。操作卡需写明，或 hnbctl 内置端口转发。
2. **operation-worker 仅接线 kubernetes provider**：`runtime-target-lifecycle-provider` 未进 RUNTIME_PROVIDERS，S3 的 Operation→Provider 链路未闭环（含 S3 遗留：readiness 自动关联 Operation、Service/HTTPRoute 删除未走 Provider）。
3. **Agent 静态 onboarding 令牌 TTL 24h**：主机集群 agent 用静态令牌（重启不失效但 24h 过期后隧道断开）。生产应走 enrollment/renewable session（AGENT_ENROLLMENT_SECRET_FILE 路径已在 target-agent.yaml 验证）。
4. **kind v1.36.1 vs 认证声明 v1.36.5**：compat 矩阵按证据写的是 1.36.5，验收环境是 1.36.1；升级 kind 节点镜像或修正矩阵，二者取一并记录。
5. portal 基础镜像为 photon 实验底座，provenance 声明待补。
6. 本机 `liteaig` 进程占用 127.0.0.1:18080-18082 段，kubectl port-forward 只绑到 [::1]——环境特定，非平台缺陷，但操作卡应提示 curl 使用 `[::1]` 或换端口。
