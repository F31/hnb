# Ubuntu worker 加入与重放验收 — 2026-10-14

- run_id：`ubuntu-worker-join-20261014`
- status：passed（限定共享安装器的真实双节点加入、重放和网络测试）
- commit：`c23f6e4128e92d7a9457807dce5b10e0b0704e73` 加未提交工作树，源码身份见下表
- command：在 `pkg/provision` 执行 `go run /tmp/opencode/control-plane-20261014/worker.go`
- started_at：`2026-10-14T16:01:27Z`（成功尝试的 worker 准备开始）
- finished_at：`2026-10-14T16:10:40Z`（网络验收资源清理及最终就绪检查）
- exit_code：0（安装器成功尝试、网络测试与清理命令）

## 环境

| 项目 | 控制面 | worker |
| --- | --- | --- |
| 公网地址 | `106.75.10.89` | `106.75.15.246` |
| 内网地址 | `10.9.36.43` | `10.9.57.29` |
| Kubernetes 节点名 | `hnb-ubuntu-cp` | `hnb-ubuntu-worker` |
| OS / 内核 | Ubuntu 24.04 / 6.8.0-138-generic | 相同 |
| Kubernetes / containerd | v1.36.5 / 2.2.1 | 相同 |
| CPU / 内存 / 根盘 | 2 vCPU / 3913 MiB / 19 GiB | 相同 |

worker 初始未发现 kubeadm/kubelet/kubectl/containerd 或所检查的 Kubernetes/HNB 配置路径；
无 swap，ubuntu 免密 sudo。worker 到 `10.9.36.43:6443` 的 TCP 检查返回 0。
控制面是同日独立安装的单控制面，见 `20261014-ubuntu-control-plane-install.md`。

## 执行与实际适配

1. 通过用户提供的账号完成 SSH 只读检查，授权已有安装专用公钥。
2. 公网 SOCKS5 SSH 握手出现超时；两次准备尝试均在执行远程安装脚本前失败。
   改为经控制面 SSH 转发到 worker 内网 `10.9.57.29:22` 后成功。未关闭防火墙或修改路由。
3. 复用 `PrereqScript` 安装精确版本并显式使用带官方 keyring 签名验证的
   Aliyun Kubernetes APT mirror；下载设置有界超时。复用 `RuntimeConfigScript`
   预先配置 SystemdCgroup、CNI 路径与显式 pause 镜像，启用 kubelet/containerd。
   准备时间 `16:01:27–16:06:02 UTC`。
4. 调用共享 `Provisioner.AddNodes`，随后对同一节点再调用一次，观察真实 Ready 后返回。
   临时 SSH runner 将控制面内网身份和 worker 名映射到受限 OpenSSH 通道，实际使用
   ubuntu/sudo；共享 AddNodes 当前默认构造 root 控制面 SSH 节点，本次环境映射不能
   当作产品原生 ubuntu SSH 路由已经验证。worker 的显式 runtime 配置也由准备阶段提供。
5. 使用保留的 smoke YAML 创建专用测试 namespace，完成网络验证后删除。

这不是平台 Intent/Operation/Provider HTTP 执行链。未伪造平台注册、观察或 Read Model。

## 加入与重放断言

| 断言 | 实测结果 |
| --- | --- |
| 首次 AddNodes | `Requested=1, Joined=1, AlreadyJoined=0`，`16:06:59 UTC` 返回 |
| 相同节点重放 | `Requested=1, Joined=0, AlreadyJoined=1`，`16:07:08 UTC` 返回 |
| 新节点 Ready | `hnb-ubuntu-worker`，v1.36.5，内网 `10.9.57.29` |
| 不重复初始化控制面 | 前后 kube-system UID、CA SHA-256、admin.conf mtime 相同 |

前后身份值：

- kube-system UID：`2b31fab0-4e35-4e14-b11e-57464fa8d869`
- CA SHA-256：`87b4b8b5266cbb24c3b1562fa31ee186b5463893ff58fca5b122f9140b62dc34`
- admin.conf mtime：`1791992249`

## CNI 与跨节点网络

初次节点 Ready 后，worker 的 Envoy 曾为 ImagePullBackOff；事件记录为 quay.io TLS
握手超时。随后 kubelet 自行重试拉取成功，不清空配置、不替换镜像，也未导入控制面缓存。
本失败状态保留，完整验收是在 Envoy rollout 成功之后执行。

测试资源：`20261014-ubuntu-node-add-smoke.yaml`。

| 路径 | 结果 |
| --- | --- |
| worker Pod `10.244.1.72` → 控制面 Pod `10.244.0.205:8080` | HTTP `hnb-provider-e2e` |
| 控制面 Pod → worker Pod `10.244.1.72:8080` | HTTP `hnb-provider-e2e` |
| worker → `server.hnb-node-add-20261014.svc.cluster.local:8080` | HTTP `hnb-provider-e2e` |
| worker 解析 `kubernetes.default.svc.cluster.local` | `10.96.0.1` |

测试 Pod 使用 `quay.io/cilium/alpine-curl:v1.10.0`，两端实际 imageID 均为
`quay.io/cilium/alpine-curl@sha256:913e8c9f3d960dde03882defa0edd3a919d529c2eb167caa7f54194528bde364`。
DNS 工具同时输出 recursion not available 提示，集群域名解析成功且命令退出码为 0。

清理后 `16:10:40 UTC`：两台节点 Ready、API readyz=ok、Cilium/Envoy 均 2/2，
operator 1/1、CoreDNS 2/2。仅删除本次创建的 `hnb-node-add-20261014` namespace。

## artifact_digests

| 文件 | SHA-256 |
| --- | --- |
| `pkg/provision/bootstrap.go` | `38bee3baf446812fa8b42dcfa921c34f85165d6ad75989bb7412d15ded393a0a` |
| `pkg/provision/orchestrator.go` | `4249b97ca3450a2c3234db90e40cc47a8b6da71e2ea09b810ba7dff5d8d1a090` |
| `pkg/provision/types.go` | `058323bed850bc7a1f41c1691fbff389fab79c4f7f8d0b0cf2935c0da089cc5e` |
| 临时 `worker.go` | `b7e4cb4e8f341eeec6592b2975bd9609b6ea8bed2df3d8ff12bfd63d5d6338c3` |
| 临时 `jump-worker.sh` | `7655db212363a602ad6575cf521baa2de1de71adba8f14fd90f1b9ef14340694` |
| smoke YAML | `366201d0ab277e02d953fbab9f4d3c5b89337308e4b2643289008ba4365a1ea5` |

临时适配器保留于 `/tmp/opencode/control-plane-20261014/`，不是发布包。
密码、私钥、kubeconfig 和 join token 不进入本报告。

## 未覆盖

平台服务凭据/SecretReference、node-add 动作级签名 conformance、正式 Intent/Operation
及 CLI → Provider 闭环、实际节点准备失败后的恢复、离线安装、重启持久化及 HA 均未由
本报告认证。7.6/7.7 的统一离线物料管理仍为后续任务；6.3 仍保持未完成。
