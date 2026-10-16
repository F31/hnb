# 1.2 安装边界决策与拒绝场景

- task: 1.2 完成 Day-0 管理基座、既有集群安装、在线托管及故障恢复的边界决策和拒绝场景
- requirement: PINST-001, DPLY-006
- date: 2026-10-11
- status: done

## 边界决策

| 模式 | 行为 | 边界 |
|---|---|---|
| `bootstrap` | 在显式声明的专用、**未纳管**管理基座上首次安装 HNB 自身控制面与平台 | 配置含 platform/target 引用、节点有其它 lifecycle owner、检测到 HNB 管理标记、无法证明未纳管 → 拒绝 |
| `existing` | 在外部管理的 Kubernetes 中安装 HNB，只拥有平台 namespace/release | 保留外部集群/CNI owner，不执行节点初始化或 reset |
| `managed` | 平台在线；create/import/upgrade/unmanage/节点变更一律走服务端 | CLI 直接执行 Provider、写平台 DB、任意 steps/commands、503 回退 SSH → 拒绝 |

关键约束：
- **平台 API 超时/503 不得触发 bootstrap fallback**。`managed` 下 API 不可达只报告连接失败并保留已知 Operation 引用。
- **bootstrap 不是通用离线 RuntimeTarget 执行器**。它只初始化 HNB 自身管理基座；业务 RuntimeTarget 写入必须经既有 ExecutionPlan/Operation。
- **bootstrap receipt 不伪造 RuntimeIntent/Operation/Read Model**；平台上线后管理关系通过正式 import Intent、Agent 认证和观测建立，receipt 仅作关联附件。
- 兼容矩阵当前只声明 KubernetesTarget 的 `create/import/upgrade/unmanage` 动作（`runtime-target/lifecycle/kubernetes`，Agent 观测源）。**没有声明 node-add 动作** → CLI 节点变更在服务端动作契约补齐前返回 `TARGET_ACTION_UNSUPPORTED`，不打印安装指令或生成 no-op Operation。

## 拒绝场景契约测试设计（待随 CLI install 落地）

| 场景 | 期望 |
|---|---|
| 配置含 `platform.targetRef` 且 mode=bootstrap | 副作用前拒绝（PINST-001） |
| 节点已有 HNB 管理标记/其它 lifecycle owner | 拒绝，指出正确管理入口（PINST-001） |
| managed 模式 API 返回 503/超时 | 保留 operationId，不触发本地 SSH/bootstrap（DPLY-006） |
| 同义 CreateKubernetesTarget 请求 | 幂等（metadata.idempotencyKey 重放）返回既有操作（DPLY-006） |
| 跨租户 SecretReference 引用 | 拒绝（DPLY-006 + CFG-002） |
| node add 尚无服务端动作契约 | `TARGET_ACTION_UNSUPPORTED`，非零退出码（DPLY-008） |

这些拒绝场景在实现 install/cluster 命令时以 Go 契约测试固定；本任务记录边界决策本身。

## 范围结论

`bootstrap` 边界当前**无法由方法级 provider 证据证明为完整平台安装**（完整 Minimal 平台 E2E 未做），因此本期**只放行 `existing`（已有 Kubernetes + HNB）路线**作为可验收交付；`bootstrap` 保留为 blocked，等 5.4/5.5 独立环境完整平台 E2E 后再开放。这与 design.md「边界不可证明则延期该路线」一致。
