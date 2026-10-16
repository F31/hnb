# 2.2 严格配置规范化与声明能力校验

- task: 2.2
- requirements: DPLY-001, DBND-002
- date: 2026-10-11
- status: done

## 实现与边界

`cmd/hnbctl/internal/toolkit/config.go`、`normalize.go`、`evidence.go`、`lock.go` 与 CLI loader：

- KnownFields、重复键与多文档拒绝；自定义 duration decoder 同样拒绝未知字段。
- IP、CIDR、公共 HTTPS origin 规范化；release lock、kubeconfig、SSH key、bundle 相对路径以配置目录为基准；支持 `~/`，规范化可重复且无文件写入。
- existing 仅接受 existing 策略和本地 kubeconfig 引用，不接受节点/网络/版本初始化输入。
- bootstrap 仅接受 provision、kubeadm `1.36.5`、Cilium `1.20.1`、native；候选拓扑上限单控制面加一个 worker。
- 拒绝缺失版本/端点、未知角色、重复角色/名称/地址、多控制面、IPv6/loopback、主机地址和 Pod/Service CIDR 冲突。
- 管理节点 API endpoint 必须绑定单控制面的 internalAddress；未证明的其他端点拓扑不放行。
- managed 配置因缺少公开 platform-context/tenant-bound credential contract 明确拒绝，不能借本地 kubeconfig/SSH 执行。
- 真实 HTTPS origin、namespace、TLS Secret、StorageClass、PG/NATS backing mode 必填；拒绝 URL 认证字段、任意路径/query、明文 DSN、bundled/external 引用混用。
- 本地凭据检查 regular-file/readability/0600 等受限权限，不打印内容、不修改权限。
- 默认读取 release lock，检查请求档位/Kubernetes 版本、声明 OS/arch 的候选证据。
- profile、route、offline 分别门控；profile 认证不能隐式认证 bootstrap/existing/离线。
- CNI 实现通过显式 plugin 字段匹配，不再从报告路径文字推断支持。

结构合法不等于平台认证。当前完整 Minimal、existing/bootstrap 和离线交付仍 blocked。
支持矩阵校验只检查输入与锁的声明；实际 OS/arch/kernel/runtime/owner、源路径连通性分别由 2.4/2.5 观测。
此任务不验证 registry 字节或发布依赖闭包（3.1/3.2），也不证明本地凭据在远端有效。

## 实际验证

在 `cmd/hnbctl` 执行，全部退出 0：

```bash
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
```

`config_contract_test.go`：24 组无效组合、重复节点/拓扑、外部 owner 输入拒绝、规范化幂等/无写入、
凭据文件内容不变及不泄漏、声明矩阵和独立认证门、未知/内联/重复/多文档拒绝。
`TestValidateLoadsDefaultLockAndBlocksUncertifiedDelivery` 验证相对锁/凭据路径、缺锁、锁不兼容与 Minimal blocked。
fixtures 的 TLS/StorageClass 名和摘要仅作结构测试，不作为实际集群/物料证据。
