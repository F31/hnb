# hnbctl 实际卸载执行器与云主机验证

- Run ID：`hnbctl-uninstall-execute-20261017`
- 工具：`hnbctl 0.2.0 (candidate)`，本地构建 linux/amd64
- 环境：`43.142.101.136`，Ubuntu 26.04，K3s `v1.36.5+k3s1`，单节点
- 配置：`/home/ubuntu/e2e/hnb.yaml`，run `5b88c013669545032676b42dcee0a64a`
- 结果：**实际删除成功**；工作负载/RBAC 删除，PVC/Secret/Namespace/
  StorageClass/TopoLVM 保留。

## 1. 变更

`hnbctl uninstall` 原仅输出计划（`executionAuthorized:false`）。新增：

- `hnbctl uninstall --execute --yes`：执行已复核的 `uninstall-hnb` 计划。
- 删除 `hnb-system` 内 Deployment/StatefulSet/DaemonSet/Service/ServiceAccount/
  Role/RoleBinding，以及 HNB ConfigMap（保留 `kube-root-ca.crt`）。
- 删除带 `app.kubernetes.io/instance=hnb` 的 ClusterRole/ClusterRoleBinding。
- 不删除 PVC、Secret、Namespace、StorageClass、TopoLVM，也不执行
  `unmanage`/`delete-management-base`；`--delete-data` 与 `--execute` 组合被拒绝。
- 删除后轮询 Pod，避免终止窗口被误报为 `REMOVAL_INCOMPLETE`。

## 2. 执行

```bash
sudo /tmp/hnbctl uninstall \
  --run-dir /home/ubuntu/e2e/runs/5b88c013669545032676b42dcee0a64a \
  -f /home/ubuntu/e2e/hnb.yaml \
  --execute --yes \
  --kubeconfig /etc/rancher/k3s/k3s.yaml \
  --instance-label hnb --format json
```

结果：`exit=0`，输出 `removal-execution/v1`，`verified=true`。

## 3. 执行后观察

| 资源 | 结果 |
|---|---|
| `hnb-system` Deployment/StatefulSet/Service/Pod | 无 |
| ClusterRole/ClusterRoleBinding `instance=hnb` | 无 |
| PVC `postgresql-data`/`nats-data` | Bound，保留 |
| Secret | 15 个保留 |
| Namespace `hnb-system` | Active |
| StorageClass `hnb-topolvm` | 保留 |
| TopoLVM controller/node | Running |

## 4. 边界

- 仅验证 existing-cluster 的 `uninstall-hnb` 本地执行；未删除数据、未删除
  TopoLVM、未执行 `unmanage`/`delete-management-base`。
- 未重新安装平台；安装执行仍使用既有 `install --execute` 证据门禁。
- 原始输出：`reports/20261017-hnbctl-uninstall-execute.json`。

## 5. 证据身份

| 文件 | sha256 |
|---|---|
| `cmd/hnbctl/internal/toolkit/removal_executor.go` | `33fc0f8b015de3a818b0e4599d286322ac05beb5d3fdc46090ceda8674434c7a` |
| `cmd/hnbctl/internal/toolkit/removal_executor_test.go` | `4141a2e1ba89ece17bacd2140234b6e7a808e2b4b32aae13093cb6e1910f6e70` |
| `cmd/hnbctl/internal/cli/uninstall.go` | `9159c49ba0b47dfe8a0974353e2a652314e074349aaa0172a470e299e1a01e94` |
| `cmd/hnbctl/internal/cli/platform.go` | `25eadf8040d0db2b23f98dcca210363f448d5306a2be8ea0222a5da258b16d40` |
| `docs/acceptance/reports/20261017-hnbctl-uninstall-execute.json` | `d87abb2755f3802e75082e6e5aa595d437480844edba3e94644d8c533a44e4bd` |
