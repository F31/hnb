# Ubuntu 24.04 SSH provider 部署真实验收

- run_id: hnb-provider-ssh-ubuntu2404-20261011
- feature: kubernetes_ssh_create
- status: passed
- acceptance_scope: 单控制面 `Provisioner.Provision()` + ubuntu/免密 sudo + RealSSH + PostgreSQL/AES-GCM + Pod/Service/DNS
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` 为基线，执行未提交工作树；最终执行路径源码身份见 SHA-256 清单
- started_at: `2026-10-11T04:53:32Z`（初次执行前的观测时间）
- finished_at: `2026-10-11T05:11:51Z`（最终重建、网络测试、清理及回归后的观测时间）
- exit_code: 0（最终干净 Kubernetes 重建和网络验收）
- artifact_digests: [20261011-provider-ssh-ubuntu2404-artifacts.sha256](20261011-provider-ssh-ubuntu2404-artifacts.sha256)

## 环境

| 项目 | 实测值 |
|---|---|
| 主机 | 106.75.26.57，内网 10.9.127.43 |
| 登录账号 | ubuntu，uid 1000，`sudo -n` 可用 |
| OS / kernel | Ubuntu 24.04 LTS / 6.8.0-31-generic，amd64 |
| CPU / 内存 | 2 vCPU / 3914 MiB |
| 根磁盘 | 19 GiB，初始可用 17 GiB |
| cgroup / swap | cgroup v2 / 无 swap |
| 节点 | hnb-ubuntu-cp，单控制面 |
| kubeadm/kubelet/kubectl | 1.36.5-1.1（v1.36.5） |
| containerd | 2.2.1-0ubuntu1~24.04.3 |
| runc | 1.3.4-0ubuntu1~24.04.1 |
| Cilium | 1.20.1 |
| Helm | v4.2.4+g3900f43（验收环境预置） |
| API endpoint | 10.9.127.43:6443 |

初始主机没有 kubeadm、containerd 或 Kubernetes 配置。首次 provider 执行完成 APT 软件包安装、containerd 配置、kubeadm 初始化和 Cilium 安装；Helm 与 SSH 公钥在验收环境准备阶段预置。

实际通过 `ubuntu` SSH 登录，由 RealSSH 使用 `sudo -n -- sh -s` 执行各阶段。报告与仓库不保留账号密码、SSH 私钥或明文 kubeconfig。

## 测试过程与首次失败

| 执行 | 结果 | 保留输出 |
|---|---|---|
| 初次部署，readiness 等待 300 秒 | **FAILED**，519.50 秒；APT、初始化和 CNI 安装成功，节点就绪门超时 | [initial-output.txt](20261011-provider-ssh-ubuntu2404-initial-output.txt) |
| 增加等待窗口后从已有初始化状态继续，`-race` | PASS，19.45 秒，kubeconfig ref version 1 | [resume-output.txt](20261011-provider-ssh-ubuntu2404-resume-output.txt) |
| 最终代码的干净 Kubernetes 重建，`-race` | PASS，156.17 秒，kubeconfig ref version 2 | [output.txt](20261011-provider-ssh-ubuntu2404-output.txt) |
| Pod HTTP / Service / DNS | PASS | [smoke-output.txt](20261011-provider-ssh-ubuntu2404-smoke-output.txt) |

首次超时原因：Cilium 镜像拉取遇到连接重置和重试。节点日志记录镜像拉取累计约 5 分钟、Cilium Pod 总启动约 5 分 9 秒，超过原 300 秒等待窗口；首次失败后各系统 Pod 自行恢复并 Ready。该失败如实保留，不作为通过证据。

最终重建前，在本次新建的测试集群执行 kubeadm reset，清理 CNI 配置/状态并重启。reset 报告一个旧 sandbox 清理超时；重启后由最终 provider readiness 和网络断言确认新集群就绪。**最终通过项是干净 Kubernetes 状态重建，保留了首次执行安装的软件包和镜像缓存，不宣称最终代码在全新 OS/空镜像缓存上一次成功。**

## 固化到代码的 Ubuntu 适配

1. 允许符合账号格式校验的非 root SSH 用户；非 root 使用非交互 sudo，sudo 不可用时正常返回失败。
2. APT 安装设置 `DEBIAN_FRONTEND=noninteractive`；GPG key 导入使用 batch/yes，支持失败后的重复执行。
3. 从 APT 目录解析所需精确 patch 对应的软件包版本，显式安装同版本 kubeadm/kubelet/kubectl。
4. readiness 门等待调整为 600 秒，仍检查 API、精确节点数、节点 Ready、所选 CNI 和 CoreDNS rollout。

采用已有 Cilium 路线：native routing、Kubernetes IPAM、自动节点路由及 BPF masquerade，Cilium 替代 kube-proxy。kubeadm 使用 `registry.aliyuncs.com/google_containers`，containerd sandbox 使用该仓库 `pause:3.10`，SystemdCgroup=true、CNI bin_dirs=/opt/cni/bin。

## 断言

| 断言 | 结果 |
|---|---|
| 真实 ubuntu SSH + sudo 执行全部 provider 阶段 | PASS |
| 精确版本安装 kubeadm/kubelet/kubectl | PASS，dpkg 及 node 版本一致 |
| API `/readyz` | PASS，ok |
| 恰为 1 个节点，hnb-ubuntu-cp Ready | PASS |
| Cilium / Cilium Envoy daemonset 1/1 | PASS |
| Cilium operator 1/1、CoreDNS 2/2 | PASS |
| kubeconfig 加密入库并经真实 PGSecretResolver 解密 | PASS |
| 同名 kubeconfig 更新，SecretReference version 1 → 2 | PASS |
| client Pod 10.244.0.242 → server Pod 10.244.0.153:8080 | PASS，HTTP 内容 hnb-provider-e2e |
| client Pod → server.hnb-ubuntu-e2e.svc.cluster.local:8080 | PASS，相同 HTTP 内容 |
| DNS kubernetes.default.svc.cluster.local | PASS，返回 10.96.0.1 |
| 清理 hnb-ubuntu-e2e namespace 后节点与系统组件仍 Ready | PASS |
| provider `go test -race ./... -count=1` / `go vet ./...` | PASS |

最终 credential reference：`local-aes/tenant/provision-ubuntu240420@2`。

清理后的组件/软件包/运行时观测：[environment-output.txt](20261011-provider-ssh-ubuntu2404-environment-output.txt)。网络测试资源：[smoke.yaml](20261011-provider-ssh-ubuntu2404-smoke.yaml)。

## 可复现命令

执行目录：`cmd/runtime-target-lifecycle-provider`。DB_DSN、HNB_MASTER_KEY、HNB_PROVISION_E2E_KEY_FILE 由验收环境提供。

```bash
export HNB_PROVISION_E2E_SPEC="$(cat ../../docs/acceptance/reports/20261011-provider-ssh-ubuntu2404-spec.json)"
go test -race ./internal/provider -run '^TestProvisionRealSSH$' -count=1 -v -timeout=22m
go test -race ./... -count=1
go vet ./...
```

## 范围限制

- 本次 Ubuntu 验收为单控制面、同节点 Pod 网络；未执行 Ubuntu worker join、Ubuntu 跨节点网络或 HA。
- API endpoint 使用内网。集成测试通过 SSH 操作集群，并验证加密 kubeconfig 内容；公网 API 及平台服务直连内网 API 的可达性未验收。
- 不覆盖北向创建 API/admission/operation-worker/RuntimeTarget 投影/Agent 注册完整链路。
- 离线安装、升级、证书轮换和 CAPI 生命周期保持原有未验收状态。
