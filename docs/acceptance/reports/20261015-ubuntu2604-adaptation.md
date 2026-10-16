# Ubuntu 26.04 适配与部署复验

## 范围与结果

Ubuntu **26.04 LTS / amd64 / 7.0.0-30-generic / cgroup v2** 的 OS 适配已实现，
并在 `43.142.101.136` 的既有单节点 K3s 上完成完整 Minimal **19 工作负载**的恢复复验。

本次验证的路线是 **existing / K3s**。Ubuntu 26.04 的 kubeadm 全新 bootstrap、
Cilium CNI、离线、HA 和真实 2C4G/15G 主机验收没有由此取得认证。
`CurrentEvidence.CertifiedOS` 保留原有 bootstrap 范围；26.04 在新增的
`OSAdaptations` 中以 `route=existing, distribution=k3s, status=verified` 登记。

## 可复核身份

- run_id：`ubuntu2604-adaptation-20261015`
- 基准 commit：`c23f6e4128e92d7a9457807dce5b10e0b0704e73`（工作树有未提交改动；不是发布标签）
- 开始：`2026-10-15T07:51:02Z`；结束：`2026-10-15T07:51:40Z`；exit_code：**0**。
- hnbctl：`sha256:ad1ac30ae67f8ab84b304391974b147685bb7f28c50e3f0d6d79e70918a66eb2`
- 验证程序：`sha256:12a386586e3d9ae3bd29f65726233c5a6aba56d039d2022c4f7ef27e65768e4c`
- [原始结果 JSON](20261015-ubuntu2604-result.json) 保留前后工作负载、Pod UID、
  镜像引用及运行时 imageID、PVC UID、迁移 ledger、API 结果和 CLI 输出；不含凭据。
- [验证程序](20261015-ubuntu2604-verify.py) 可复用，默认只读；本次显式启用工作负载重启。

## 环境

| 项目 | 实测 |
|---|---|
| 主机 | 4 vCPU / 7680Mi 内存 / 约 118Gi 根文件系统 |
| OS | Ubuntu 26.04 LTS（不是 TencentOS/CentOS） |
| kernel / arch | 7.0.0-30-generic / amd64 |
| APT / systemd | 3.2.0 / 259.5-0ubuntu3 |
| AppArmor | active，未关闭或修改策略 |
| lvm2 | 2.03.31-2ubuntu3 |
| Kubernetes | K3s v1.36.5+k3s1 |
| cgroup | cgroup2fs |
| swap | 1 个活动 swap 文件；K3s 既有配置显式允许；本次未修改 |
| 存储 | TopoLVM 0.42.0 / chart 17.3.0；临时 8Gi loop VG，仅作功能测试 |

代理为操作者已有的 `127.0.0.1:7892` HTTP CONNECT；凭据只保留在私有临时目录，
没有写入仓库，也没有修改系统代理配置。

## 适配与 hnbctl 解耦

1. `pkg/provision/os.go` 集中描述发行版的包管理器、默认 SSH 用户和 bootstrap
   依赖。查询返回防御性副本，CLI 与 recipe 不再各自维护 OS 白名单。
2. 26.04 的 APT 3 自带 HTTPS，APT 引导不再依赖 `apt-transport-https`。
   24.04 和 Rocky 10.1 的已有配置保留；包选择在**目标机**读取 `/etc/os-release`，
   不依据安装器本机 OS。实际安装 kubeadm 的分支本次未在已有 K3s 主机执行。
3. `OSAdaptations` 将内核/架构/路线/发行版证据与一般 OS 识别分开。
   26.04 不可静默复用 24.04 的 release lock，未知内核/架构和已有 owner 的隔离仍有效。
4. 新增无需配置文件的 `hnbctl os` 与 `hnbctl os inspect`，JSON 版本分别为
   `os-catalog/v1`、`host-os/v1`。本机观察复用 SSH 预检的固定程序和有界解析器；
   不执行包安装、swap 修改、集群重置或权限变更。
5. 支持 `--help`、`-h`、`<command> --help`、`--version`；保留原有 `help`、
   `version`、配置文件、退出码与命令入口。

观察结果中 `adapted=true, osKernelVerified=true, bootstrapCertified=false` 是预期结果：
报告确认本机兼容范围，而非授予 kubeadm/bootstrap 安装权限。

## 部署复验步骤与断言

```bash
# 在节点上，以可读取本次评估凭据的账户执行（凭据不作为命令参数）
sudo python3 hnb-ubuntu2604-verify.py \
  --hnbctl /home/ubuntu/hnbctl-ubuntu2604 \
  --restart-workloads \
  --output /home/ubuntu/hnb-ubuntu2604-result.json
```

| 断言 | 结果 |
|---|---|
| 本机 OS/kernel/cgroup 正确识别，既有 K3s owner 和 swap 被如实报告 | 通过 |
| Ubuntu 26.04 草稿生成，用户 ubuntu、权限 0600，不执行安装 | 通过 |
| PostgreSQL、NATS 依次重启，17 个 Deployment 全部滚动重建 | 通过 |
| 前后均恰好 19 个当前 Ready 工作负载；控制器 observedGeneration 匹配 | 通过 |
| 19 个工作负载的 Pod UID 全部更换 | 通过 |
| PG 2Gi / NATS 1Gi，TopoLVM StorageClass、PVC UID 与 PV 引用前后不变 | 通过 |
| platform / market / artifact 迁移记录前后各 103 条 | 通过 |
| Portal 登录、workspaces、tenants、agents API | 通过 |
| Agent 隧道重连、匿名 workspaces 请求拒绝 | 通过 |

结束时节点采样：**948m CPU / 1549Mi 内存**；TopoLVM controller 7m/48Mi、
node 3m/23Mi。节点指标已包含存储组件，不能重复相加；这是滚动恢复后的采样，
不是持续采集的峰值或 2C4G 压测结论。

## 回归

- `cmd/hnbctl`：`go test -race ./... -count=1`、`go vet ./...`。
- `pkg/provision`：`go test -race ./... -count=1`、`go vet ./...`。
- `cmd/runtime-target-lifecycle-provider`：`go test ./...`、`go vet ./...`。
- 覆盖 24.04/26.04/Rocky 适配、目标机 APT 选择、未知 OS/内核/架构拒绝、
  24.04 lock 与 26.04 节点不匹配、owner 保留、版本化 JSON 与简写帮助。

## 后续范围

- 全新 Ubuntu 26.04 的 kubeadm/containerd/Cilium 装机需要独立空白节点；不能在
  当前已有 K3s 节点上执行破坏性接管来取得结果。
- loop LVM 后端没有完成整机重启及自动重挂载验证；本次只重建工作负载。
- Agent 使用已有接入流程签发的限时凭据；服务凭据轮换和长期运行另行验收。
- `Minimal` 完整发布、`hnbctl install` 端到端自动执行、HA、离线和 2C4G/15G
  的全盘预算继续保留各自的未完成状态。
