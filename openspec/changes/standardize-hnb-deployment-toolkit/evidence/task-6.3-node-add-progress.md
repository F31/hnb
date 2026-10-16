# 6.3 节点增加实现进度（部分交付）

日期：2026-10-14。任务保持未完成。

## 已实现与验证范围

- DPLY-006：`hnbctl node add` 通过正式 RuntimeIntent 入口提交
  `AddNodesRuntimeTarget`，输出服务端 Intent/Operation 引用。平台不可达时返回
  连接失败；CLI 边界测试验证无本地执行或伪造 Operation 输出。
- DPLY-004：请求包含目标 ID、`expectedVersion`、幂等键和 SSH SecretReference。
  本轮修复此前 CLI 缺失 `expectedVersion` 的契约缺口，TLS fixture 成功路径
  明确断言版本 3 被序列化发送。
- DPLY-008：帮助包含必要的平台、身份、版本和请求参数；声明文件使用严格 YAML
  字段解码且仅接受单文档；节点限定为 1..100 个 worker、RFC-1123 子域名，拒绝
  重复、额外节点字段及额外凭据字段。解析错误不回显原始 YAML 值。
- Provider 单元测试覆盖 SSH node-add 参数映射、缺少已记录 endpoint 的 409、
  委派失败的可重试 500、非法角色 400、创建后的 namespace 记录持久化及新 manager
  从共享 fake Kubernetes namespace 恢复记录。它们使用 fake provisioner/client，
  不等同于真实节点准备或加入后的 Ready 观测。

## 命令与结果

在 `cmd/hnbctl`：

```sh
go test ./...
go vet ./...
```

退出码均为 0；CLI 与 toolkit 包测试通过，vet 无诊断。

在 `cmd/runtime-target-lifecycle-provider`：

```sh
go test ./...
```

退出码 0；根包与 provider 包测试通过（缓存结果）。本记录不声称已完成整个
Go workspace 回归；workspace 根目录没有对应 `./...` 模块。

## 候选 CLI 用法

声明文件仅包含凭据引用和节点身份（示意值，需替换为实际已登记引用）：

```yaml
sshSecretRef:
  provider: vault
  scope: tenant-a
  name: cluster-ssh
  version: "2"
nodes:
  - name: worker-3
    role: worker
```

```sh
hnbctl node add --target-id <target-uuid> --expected-version <current-version> \
  -f nodes.yaml --platform-url <https-platform-origin> --token-file <private-token-file> \
  --tenant-scope <tenant-scope> --idempotency-key <stable-key> \
  --correlation-id <request-uuid> --format json
```

提交成功表示服务端接受请求，不表示节点已经 Ready。服务端仍按能力矩阵决定准入。

## 剩余条件

- DBND-002：后续 Ubuntu 双节点已完成共享安装器真实加入、已加入节点重放、
  控制面身份不变及 Ready/跨节点网络检查，见
  `docs/acceptance/reports/20261014-ubuntu-worker-join.md`。
  正式平台执行链、实际准备失败恢复及 Rocky 独立扩容证据仍缺。
- 签名 conformance 校验目前限定 `KubernetesTarget/import`；node-add 的受信报告、
  登记与运行物料绑定尚未完成。不能仅凭上述单元测试将矩阵翻为 REQUIRED。
- 6.3 的真实验收及 worker 绑定完成后，方可判定任务完成。

## 连续扩容记录继承回归

新增 `TestKubernetesManagerConsecutiveNodeAddsPreserveClusterIdentity`，按真实
node-add 请求形状省略 `desiredVersion`。修复前第一次执行成功、第二次执行失败：
`node add requires the cluster's desired version to be recorded`。

原因：Apply 在完成 node-add 后用输入中的空版本替换了目标记录，且未继承
`kubeconfigRef`，导致连续扩容丢失版本和后续持久化使用的凭据引用。
修复后 node-add 从已有目标记录继承版本及 kubeconfig 引用；测试验证连续两次扩容、
每次扩容的精确重放不重新委派 join，以及 namespace 最终版本/引用/fencing 保留。
node-add 测试辅助函数也已移除默认版本，避免再次掩盖真实契约。

本轮在 `cmd/runtime-target-lifecycle-provider` 执行 `go test ./...` 和
`go vet ./...`，退出码均为 0。该回归为 fake client/provisioner 自动测试，
不替代真实节点重放与观测验收。
