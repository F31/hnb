# 1.5 Minimal T0/T1 依赖清单与 Chart 缺口

- task: 1.5 形成 Minimal T0/T1 实际依赖清单，列出 Chart 缺失组件与每平面数据库/制品绑定
- requirement: PINST-002, PINST-003, DBND-004
- date: 2026-10-11
- status: done

## Minimal 平台组件依赖 DAG

```text
namespace + StorageClass
  ├─ PostgreSQL (bundled 单实例或 external secretRef)
  └─ NATS JetStream (bundled 文件持久化单实例或 external secretRef)
        ├─ 身份/KMS 材料 (签名密钥、manifest、KMS AES key、服务 token)
        └─ owner-lock 数据库迁移 (每个 plane 独立 DB/账号/owner)
              ├─ apiserver (T0)            ── console API、字典、资源投影
              ├─ operation-worker (T0)     ── ExecutionPlan→Operation 执行
              ├─ platform-api (T1)         ── BFF 聚合
              ├─ extension-controller (T1)
              ├─ app-market (T1)           ── 应用市场/插件执行器
              ├─ plugin-worker (T1 缺失)    ── 插件运行时执行
              ├─ runtime-target-lifecycle-provider (T1 缺失) ── 集群生命周期 Provider
              ├─ gateway-provider (T1)
              ├─ cluster-router / tunnel-server (T1) ── 控制链路
              ├─ rbac-syncer (T1)
              ├─ alert-manager (T1)
              ├─ cluster-agent (T1)        ── Agent 观测
              ├─ network-registry + cilium-provider (T1, 网络)
              └─ Portal (T1 缺失)          ── Web 控制台
```

可选（默认关闭，独立 Pack 门控）：calico/kube-ovn/karmada/edge-provider/ai-extension。

## Chart 现状与缺失组件（`deploy/charts/hnb/charts/`）

| 缺失/缺口 | 说明 |
|---|---|
| **Portal（web）** | `web/` monorepo 存在但无 portal 子 Chart，无 Service/Ingress 交付 |
| **plugin-worker** | 无子 Chart；应用市场执行能力缺运行容器 |
| **runtime-target-lifecycle-provider** | 无子 Chart；本 change 候选源码有实现但未进 Chart |
| `global.imageTag: latest` | 未锁定真实 digest（DBND-001 要求拒绝） |
| `global.postgresql.dsn` 示例 `postgres://postgres:postgres@...` | 生产禁止示例共享密码/跨平面全局 DSN |
| NATS mTLS 需手工装配 secret | 身份/KMS/服务凭据需工具生成或 existingSecret |

## 每平面数据库/制品绑定

| 平面 | 数据库 | 制品 | 说明 |
|---|---|---|---|
| 平台运行治理 | 平台 DB（独立账号/owner） | OCI/helm | Operation/审计/身份事实源 |
| 应用市场 | 市场 DB（独立） | OCI Chart 包 | 不共享平台表，市场/插件执行器独立身份 |
| 制品存储 | 制品 DB（独立）+ 对象 | OCI 内容寻址 | API 不代理大文件 |
| AI/Edge | 各自可选 | Pack 门控 | Minimal 默认关闭 |

CLI 不共享任何平面数据库、不引入 DSN 到工具、不代理大文件；跨平面仅通过公开契约。

## 验证命令

```bash
# 现有 provider 参数映射核对（toolkit network/image params → provider parameters）
rg -rn "parameterString" cmd/runtime-target-lifecycle-provider/internal/provider/kubernetes.go
# Chart 依赖核对
helm dependency list deploy/charts/hnb
```
