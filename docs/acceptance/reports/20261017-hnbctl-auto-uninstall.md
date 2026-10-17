# hnbctl 零配置卸载与 -all/-data 全清验证

- Run ID：`hnbctl-auto-uninstall-20261017`
- 工具：`hnbctl 0.2.0 (candidate)`，linux/amd64
- 环境：`43.142.101.136`，Ubuntu 26.04，K3s `v1.36.5+k3s1`
- 结论：**零配置自动检测与 `-all -data` 全量清理成功**。

## 1. 简化后的命令

| 命令 | 行为 |
|---|---|
| `hnbctl uninstall` | 自动解析 kubeconfig、检测 HNB，确认后删除 HNB 工作负载/RBAC，保留 PVC/Secret/namespace/存储组件 |
| `hnbctl uninstall -all` | 再删除 `topolvm-system`、`hnb-topolvm`、`topolvm.io` CSIDriver、TopoLVM cluster RBAC；保留数据 |
| `hnbctl uninstall -all -data` | 再删除 PVC、PV、LogicalVolume、Secret、namespace 和 LV CRD |
| `hnbctl uninstall --plan` | 只输出计划 |
| `hnbctl uninstall --yes` | 非交互确认 |
| `hnbctl uninstall --kubeconfig PATH` | 指定 kubeconfig；默认自动尝试 `KUBECONFIG`、`~/.kube/config`、`/etc/rancher/k3s/k3s.yaml` |
| `hnbctl uninstall --namespace NAME` | 覆盖默认 `hnb-system` |

自动检测优先查 `hnb-system` 的 `instance=hnb` Deployment；namespace 已删但仍有
`hnb-topolvm` StorageClass、`topolvm-system` 或 LV CRD 时也识别为残留。

旧记录式 `--installation`/`--run-dir -f` 仍保留；`unmanage` 和
`delete-management-base` 仍不在本地执行。

## 2. 执行结果

```bash
sudo /tmp/hnbctl uninstall -all -data --yes --format json
```

`exit=0`，`removal-execution/v1`，`verified=true`。删除：

- `hnb-system`/`topolvm-system` namespace
- PVC、PV、4 个 LogicalVolume
- 15 个 Secret
- `hnb-topolvm` StorageClass、`topolvm.io` CSIDriver
- TopoLVM cluster RBAC 与 `logicalvolumes.topolvm.io` CRD
- HNB 工作负载与 `instance=hnb` ClusterRole/Binding

执行后主机仅剩：

- namespaces：`default`、`kube-node-lease`、`kube-public`、`kube-system`
- PV/LV：0
- StorageClass：`local-path`
- CSIDriver/TopoLVM CRD：0
- Pods：CoreDNS、local-path-provisioner、metrics-server

K3s、CNI、OS 与外部业务资源未删除。

## 3. 边界

- `-all` 不含数据：PVC/PV/LV/Secret/namespace 保留，但 TopoLVM/SC 被删除。
- `-data` 不含 `-all`：数据删除，但 TopoLVM/SC/CRD 保留（无数据 LV）。
- 主机目录（如 `/home/ubuntu/e2e`、`/var/lib/hnb-minimal-eval`）不由 hnbctl 管理，
  本命令未删除。
- 未重新安装；`install --execute` 路径未在本次验证。

## 4. 证据身份

| 文件 | sha256 |
|---|---|
| `cmd/hnbctl/internal/cli/auto_uninstall.go` | `6cb18f9ed32bcac4a7eb14448f14cd746e0929189b32694775def2701659f0d2` |
| `cmd/hnbctl/internal/cli/auto_uninstall_test.go` | `c78c1170f7be6ee7d2f80c3879d7e369d046e97414ab5fa1252d3ac1b1ee0dbf` |
| `cmd/hnbctl/internal/toolkit/removal.go` | `d4b4020e9ae9b20523c74447958f3cab4f42d6a0334b30b7bc8b938159498e2e` |
| `cmd/hnbctl/internal/toolkit/removal_executor.go` | `cbaf1d0365bb06ba9ea27ef1b9ab049b440f8940b15dfed57ff8ed251a66a354` |
| `cmd/hnbctl/internal/toolkit/removal_executor_test.go` | `2d44964839fb5dd173a4750c23d86460fd4c9591698fbe0bb9d78c44667a2dba` |
| `cmd/hnbctl/internal/cli/uninstall.go` | `59b7471fc5deb7f18101b74429de09293768d0626a617848cf189ad7ac6bbaad` |
| `docs/acceptance/reports/20261017-hnbctl-auto-uninstall.json` | `88c709aef019125b643a8d83de06d8811643b99fcb0ab3095735df619b405dab` |
