# Ubuntu 单控制面安装 — 2026-10-14

## 结果与范围

按用户“重新安装控制面”的要求，将原候选 worker `106.75.10.89` 改作控制面。
执行前未发现已有 Kubernetes 初始化或所检查的 HNB 管理路径；本次没有执行 reset。
单控制面 Kubernetes、Cilium 和 CoreDNS 就绪检查通过。

本报告是独立在线 Kubernetes 安装记录，不是 HNB 平台安装、正式 RuntimeTarget
纳管、node-add conformance 或扩容验收。主机已不再作为待加入 worker。

## 环境与安装配置

| 项目 | 实测值 |
| --- | --- |
| 公网 SSH / 账号 | `106.75.10.89` / `ubuntu`，免密 sudo |
| 内网 API endpoint | `10.9.36.43:6443` |
| 节点名 | `hnb-ubuntu-cp` |
| OS / 内核 | Ubuntu 24.04 LTS / 6.8.0-138-generic，x86_64 |
| 资源 | 2 vCPU / 3913 MiB / 根盘 19 GiB |
| Kubernetes | kubeadm、kubelet、kubectl `1.36.5-1.1`，节点 `v1.36.5` |
| runtime | containerd `2.2.1-0ubuntu1~24.04.3`，runc `1.3.4-0ubuntu1~24.04.1` |
| Helm / CNI | Helm `v4.2.4+g3900f43` / Cilium `1.20.1` |
| 网络 | Pod `10.244.0.0/16`、Service `10.96.0.0/12`，Cilium native routing、Kubernetes IPAM、BPF masquerade、kube-proxy replacement |
| 安装后资源 | 根盘使用约 4.6 GiB、剩余约 14 GiB；内存 available 2683 MiB |

SSH 通过本地 `127.0.0.1:7892` SOCKS5 隧道，服务器 host key 已固定到本地
known-hosts 文件。专用 SSH 密钥在受限临时目录生成并授权到该节点；不保留密码到报告。

## 执行路径与实际来源

复用工作树 `pkg/provision.Provisioner.Provision` 的 prereq、containerd 配置、
kubeadm init、CNI、cluster-acceptance、read-kubeconfig 阶段。临时 Go 适配器
用 OpenSSH runner 接入本地 SOCKS5 路径；输出仅记录阶段事件，不打印 join token 或 kubeconfig。

- Kubernetes 镜像及 pause：显式 `registry.aliyuncs.com/google_containers`。
- Cilium Chart：显式 `oci://quay.io/cilium/charts/cilium`，版本 `1.20.1`。
- Cilium 镜像：显式 `quay.io/cilium`；临时在线适配器将共享脚本的平台镜像别名
  `cilium-operator` 替换为官方 `operator-generic`。该适配未修改共享包源码。
- Helm：`get.helm.sh/helm-v4.2.4-linux-amd64.tar.gz`，与同源 SHA-256 校验文件核对：
  `c306b46f719b0a4da32d0f78ee21bf90ce8d602f15b22ab753f0674d1670a7f3`。

### 失败与恢复条件

首次适配器输入缺少显式 OCI Chart repository，在 plan 校验阶段拒绝，无节点初始化。
补齐来源后，原始 pkgs.k8s.io 路径的软件包下载停滞，安装等待超时；远端当时仍有
本次 APT 下载进程。确认尚未进入 kubeadm init 后，终止该进程并检查无 dpkg 运行。
尝试临时代理下载也未完成所需依赖。

随后将本次创建的 Kubernetes APT source 切换到
`https://mirrors.aliyun.com/kubernetes-new/core/stable/v1.36/deb/`，保留官方
`signed-by` keyring。APT 的 InRelease/索引校验正常，剩余 52.4 MB 软件下载用时
3 分 52 秒，精确版本安装成功。显式启用 kubelet/containerd 后，重新运行共享安装器。
成功运行复用了此前已下载的软件包，不能表述为最终执行在全新 OS/空缓存上一次成功。

## 成功阶段时间（UTC）

| 阶段 | 开始 | 完成 |
| --- | --- | --- |
| 整体成功运行 | 15:36:40 | 15:38:56 |
| kubeadm init | 15:36:46 | 15:37:24 |
| CNI | 15:37:24 | 15:37:33 |
| cluster acceptance | 15:37:33 | 15:38:55 |
| kubeconfig 读取及本地加密保存 | 15:38:55 | 15:38:56 |

## 后验收（15:39:34 UTC 起）

- `kubectl get --raw=/readyz`：`ok`。
- `hnb-ubuntu-cp`：Ready，control-plane，`v1.36.5`，IP `10.9.36.43`。
- Cilium、Cilium Envoy daemonset：均 `1/1` Ready。
- Cilium operator：`1/1`；CoreDNS：`2/2`。
- etcd、apiserver、controller-manager、scheduler：均 `1/1 Running`，0 次重启。
- containerd/kubelet：均 active、enabled。
- Helm release：`cilium`，revision 1，`deployed`，chart/app `1.20.1`。
- kube-system namespace UID：`2b31fab0-4e35-4e14-b11e-57464fa8d869`，可用于后续身份核验。
- `/etc/kubernetes/admin.conf`：`0600 root:root`。本地副本以 AES-GCM 加密保存在
  `/tmp/opencode/control-plane-20261014/`，密钥及密文文件均 `0600`，目录 `0700`。
  这是本地独立安装凭据，不是平台注册的 SecretReference；远端 admin.conf 是持续保留的管理入口。

## 运行镜像身份

| 组件 | 实际 container imageID 的 SHA-256 |
| --- | --- |
| Cilium | `ae9ea21f7427fe24bc6ea7247eb552157a1b0a431744045d3f641545ca71d11b` |
| Envoy | `75b8094c7127736a2ffd2dce3945e0931cb6df21b0372ff661940eca26730b91` |
| operator-generic | `6c3885fc7b629099fdbe2a5c87869c86feb825fa18fae299eac0f61918d16ecf` |
| CoreDNS | `89e5ad786ad249d11b1fdc37460fb36d1f5385ba077a3adde2a028cb6d82c60e` |
| apiserver | `d85fa894a23cefb8d0fc2f0b0edb19233800a844448cd823b7173cefb41206af` |

## 执行源码身份

基线仍为 `c23f6e4128e92d7a9457807dce5b10e0b0704e73` 加未提交工作树。

| 文件 | SHA-256 |
| --- | --- |
| `pkg/provision/bootstrap.go` | `38bee3baf446812fa8b42dcfa921c34f85165d6ad75989bb7412d15ded393a0a` |
| `pkg/provision/orchestrator.go` | `4249b97ca3450a2c3234db90e40cc47a8b6da71e2ea09b810ba7dff5d8d1a090` |
| `pkg/provision/types.go` | `058323bed850bc7a1f41c1691fbff389fab79c4f7f8d0b0cf2935c0da089cc5e` |
| 临时适配器 `/tmp/opencode/control-plane-20261014/main.go` | `f2e6fe935a3d44667f25d7681bc11f53aeef65639ca515d86e2d307af3060be4` |

## 使用与后续

在该主机以 ubuntu 登录后：

```sh
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get nodes -o wide
```

下一步需要另一台空白 worker，先核验其到 `10.9.36.43:6443` 及 Cilium native routing
所需节点/Pod 路径可达。尚未执行跨节点网络测试、节点重启测试、平台纳管或正式 node-add。
