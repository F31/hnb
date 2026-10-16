# 双节点 SSH provider 真实装机验收

- run_id: hnb-provider-ssh-rocky10-20261011
- feature: kubernetes_ssh_create
- status: passed
- acceptance_scope: `Provisioner.Provision()` + RealSSH + PostgreSQL/AES-GCM secret bridge + worker join + 跨节点网络
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` 为基线；执行了未提交的工作树改动，确切源码身份见 SHA-256 清单
- started_at: `2026-10-11T04:25:09Z`（最终干净重建前的观测时间）
- finished_at: `2026-10-11T04:38:48Z`（网络验收、清理和最终幂等/race 验收后的观测时间）
- exit_code: 0（干净重建、跨节点 smoke 和最终幂等/race 执行均成功）
- artifact_digests: [20261011-provider-ssh-rocky10-artifacts.sha256](20261011-provider-ssh-rocky10-artifacts.sha256)

## 环境与边界

| 项目 | 控制面 | worker |
|---|---|---|
| SSH 地址 | 106.75.8.151 | 106.75.13.7 |
| 内网地址 | 10.9.31.180 | 10.9.23.181 |
| 节点名 | hnb-e2e-cp | hnb-e2e-worker |
| 系统 | Rocky Linux 10.1 | Rocky Linux 10.1 |
| 内核 | 6.12.0-124.8.1.el10_1.x86_64 | 同左 |
| Kubernetes | v1.36.5 | v1.36.5 |
| containerd | 2.2.7 | 2.2.7 |
| swap | 无 | 无 |

- 控制面装机工具已有预置；Helm `v4.2.4+g3900f43` 由验收环境预置。provider 显式要求 Cilium 路线有 Helm。
- worker 初始没有 Kubernetes，provider 的前期真实执行安装了 kubeadm/kubelet/kubectl/containerd，并完成 join。最终干净重建保留软件包与镜像缓存，重置的是 Kubernetes 状态，**不是全新 OS 安装验收**。
- 两个节点均执行 `kubeadm reset --force`，清理测试 CNI 配置/状态并重启，再运行保留输出对应的干净重建。reset 曾报告部分旧 sandbox 清理超时；后续重启并由真实 readiness/网络断言确认新集群可用。
- 数据库为本地 PostgreSQL；补齐 migration 101 前确认重复分组及 NULL scope/purpose 均为 0，执行后 UPDATE/DELETE 均为 0。
- `registry.k8s.io` 出网不可用。kubeadm 镜像仓库和 containerd pause 镜像均显式使用 `registry.aliyuncs.com/google_containers`。
- CNI 为 Cilium `1.20.1`，`kubeProxyReplacement=true`，native routing、自动直连节点路由、Kubernetes IPAM 和 BPF masquerade。provider 不安装 kube-proxy；不依赖精简内核缺失的 iptables/br_netfilter 模块。
- API endpoint 为 `10.9.31.180:6443`，两个云节点可达。本地验收执行器通过 SSH 调用集群；公网 `106.75.8.151:6443` 在本次检查中无法连接，**未验收本地 provider 通过存储 kubeconfig 直连 API 的可达性**。部署时 provider/Agent 需要可达内网 endpoint。
- 本报告不覆盖北向创建 API → admission → operation-worker → RuntimeTarget 投影/Agent 注册的完整链路，也不认证 HA、离线安装、升级或 CAPI 生命周期。

## 可复现输入与命令

输入：[20261011-provider-ssh-rocky10-spec.json](20261011-provider-ssh-rocky10-spec.json)。执行目录为 `cmd/runtime-target-lifecycle-provider`。

```bash
# DB_DSN、HNB_MASTER_KEY 和 HNB_PROVISION_E2E_KEY_FILE 由验收环境提供。
# key file 是被节点授权的 SSH 私钥；报告与仓库不保留明文私钥或密码。
export HNB_PROVISION_E2E_SPEC="$(cat ../../docs/acceptance/reports/20261011-provider-ssh-rocky10-spec.json)"
go test ./internal/provider -run '^TestProvisionRealSSH$' -count=1 -v -timeout=22m

# 后续幂等执行，同时检查真实 SSH stdout/stderr 并发写入：
go test -race ./internal/provider -run '^TestProvisionRealSSH$' -count=1 -v -timeout=22m

# 常规回归：
go test -race ./... -count=1
go vet ./...
```

集成测试通过真实密钥桥将 SSH key 加密入库，解析后调用 RealSSH；不替换 SSH runner、版本校验或 StoreKubeconfig。成功后通过 PGSecretResolver 解密存储的 kubeconfig，验证 endpoint、客户端证书及密文存储。

## 保留输出与断言

- 干净重建：[20261011-provider-ssh-rocky10-output.txt](20261011-provider-ssh-rocky10-output.txt)，`PASS`，91.00 秒。
- 最终工作树的幂等/race 验收：[20261011-provider-ssh-rocky10-resume-output.txt](20261011-provider-ssh-rocky10-resume-output.txt)，`PASS`，89.36 秒。
- 网络 smoke manifest：[20261011-provider-ssh-rocky10-smoke.yaml](20261011-provider-ssh-rocky10-smoke.yaml)。
- 网络输出：[20261011-provider-ssh-rocky10-smoke-output.txt](20261011-provider-ssh-rocky10-smoke-output.txt)。

| 断言 | 结果 |
|---|---|
| `/readyz` 成功、节点数恰为 2、两个节点 Ready | PASS |
| worker 实际 kubeadm join | PASS |
| Cilium daemonset 2/2、Cilium operator 1/1 | PASS |
| Cilium Envoy daemonset 2/2 | PASS（清理后的观察） |
| CoreDNS rollout，2/2 Running | PASS |
| kubeconfig 加密入库，并通过真实解析器解密 | PASS |
| 幂等执行后 kubeconfig SecretReference version 2 → 3 | PASS |
| worker Pod `10.244.1.202` → 控制面 Pod `10.244.0.227:8080` | PASS，HTTP 内容 `hnb-provider-e2e` |
| worker Pod → `server.hnb-provision-e2e.svc.cluster.local:8080` | PASS，相同 HTTP 内容 |
| 集群 DNS `kubernetes.default.svc.cluster.local` | PASS，返回 `10.96.0.1` |
| 清理 `hnb-provision-e2e` namespace 后两个节点仍 Ready | PASS |
| provider 全模块 `-race` 回归及 `go vet` | PASS |

最终 credential reference：`local-aes/tenant/provision-20261011prov@3`。其内容不在报告中展示。

## 真实执行发现并修复的问题

1. 旧 readiness 报告只证明手工配置的 Cilium 集群可用；旧 provider 验收检查 Flannel，在该集群上因 namespace 不存在失败。本报告替代其作为 provider 装机路径的验收依据，保留旧报告作为历史记录。
2. 本地 DB 缺少 migration 101 唯一索引，SecretReference 的 ON CONFLICT 失败；补齐后通过。
3. SSH stdout/stderr 并发写 `bytes.Buffer` 损坏 join 标记；改为加锁 writer，不暴露 `bytes.Buffer.ReadFrom` 的绕过路径，并通过并发回归/race 验证。
4. worker containerd 默认 pause 镜像不可拉，CNI bin_dir 与 Cilium 不一致；通过显式 sandboxImage 与命名配置 import 修复，保留其他运行时配置。
5. 改变 containerd cgroup 驱动后，运行中的 kubelet 缓存旧驱动；配置步骤同步重启已有 kubelet，确保双方一致。
6. 首版 smoke server 使用了该镜像未提供的 busybox httpd applet；改为镜像实际提供的 nc，加入 TCP readiness 后重新验证。失败 smoke 不作为通过证据。

## 字典接口旁路验证

本地 apiserver `:38080` 的登录与 Bearer 请求 `/api/v1/dictionaries/cluster.versions` 返回 HTTP 200，items 包含 `code=1.36.5`、`integrationTestStatus=passed`、`offlineBundleAvailable=false`。这是版本字典验证，不是完整北向创建流程验收。
