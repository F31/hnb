# 1.3 部署/物料/预检/收据/诊断 Schema

- task: 1.3 定义 DeploymentConfig、ReleaseLock、PreflightReport、BootstrapReceipt 与诊断 Schema
- requirement: DPLY-001, DPLY-005, DBND-001
- date: 2026-10-11
- status: done

## 交付物

新增 Go 模块 `cmd/hnbctl`（workspace 注册），核心包 `internal/toolkit`：

| 文件 | 内容 |
|---|---|
| `config.go` | `DeploymentConfig`（`deployment.hnb.io/v1alpha1`）：mode/strategy/profile/cluster/platform/artifacts/timeouts，严格 `decodeStrict` 拒绝未知字段，`Validate` 覆盖拓扑/角色/CIDR/凭据/版本 |
| `lock.go` | `ReleaseLock`（`release-lock/v1`）：images/charts/nodeArtifacts/migrations/compatibility，拒绝 latest、占位摘要、缺失摘要 |
| `evidence.go` | `EvidenceMatrix` + `EvidenceGate`：profile/版本/架构证据门控，与结构校验分离（编辑草稿 vs 安装放行） |
| `report.go` | `PreflightReport` + `PreflightCheck`（pass/warn/blocked、expected/actual、reasonCode/remediation） |
| `receipt.go` | `BootstrapReceipt` + 稳定 `PhaseName` 列表（draft→succeeded/failed/blocked） |
| `diagnosis.go` | `DiagnosisBundle` + `Redact`/`RedactMap`（密码/私钥/kubeconfig/DSN/NATS token 脱敏，敏感 key 整值丢弃） |
| `strict.go` | KnownFields 严格解码 + 拒绝多余 YAML 文档 |

## 测试证据

`internal/toolkit/validate_test.go` 与 `testdata/` fixtures：

| 用例 | 结果 |
|---|---|
| 未知字段拒绝 | PASS（`notARealField` 被拒绝） |
| bootstrap-minimal 合法通过 | PASS |
| existing-minimal（外部 PG/NATS、离线 bundle）合法通过 | PASS |
| StandardHA 未认证被 EvidenceGate 拒绝 | PASS |
| 多控制面节点拒绝 | PASS |
| 未认证 kube 版本（1.35.0）拒绝 | PASS |
| podCIDR/serviceCIDR 重叠拒绝 | PASS |
| 空 publicBaseURL 拒绝 | PASS |
| managed+provision 拒绝 | PASS |
| flannel 网络拒绝 | PASS |
| ReleaseLock latest+占位摘要拒绝 | PASS |
| ReleaseLock 真实摘要通过 | PASS |
| 脱敏（hunter2/DSN/私钥/kubeconfig） | PASS |
| RedactMap 敏感 key 整值丢弃 | PASS |
| Preflight blocking count | PASS |
| fixture 往返（bootstrap/existing 通过、standard-ha 拒绝） | PASS |

```bash
cd cmd/hnbctl && go test -race ./... -count=1 && go vet ./...
# ok ...  1.034s ; ALL_OK
```

## 边界说明

- 结构校验 `Validate` 与证据门控 `EvidenceGate` 分离：`init` 生成的草稿允许编辑，`install/resume` 先过 EvidenceGate 拒绝未认证 profile/架构。当前 evidence 中 Minimal 完整平台尚未认证（blocked），因此 bootstrap-minimal fixture 通过结构校验但不代表可安装成功。
- CLI 初始配置/receipt 不新建事实数据库；在线事实仍复用 owning plane 既有 Operation/审计。
