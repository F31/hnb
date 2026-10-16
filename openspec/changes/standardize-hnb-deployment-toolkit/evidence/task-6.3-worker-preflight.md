# 6.3 新 worker 只读预检：等待可达目标集群

日期：2026-10-14。状态：blocked，尚未执行节点增加。

后续用户要求重新安装控制面，该主机已改作控制面并安装成功。
本文件保留安装前的历史预检，当前状态见
`docs/acceptance/reports/20261014-ubuntu-control-plane-install.md`。

## 主机与访问

- 用户提供的候选 worker：`106.75.10.89`，账号 `ubuntu`。
- SSH 实测通过本地 `127.0.0.1:7892` SOCKS5 隧道完成密码认证；
  直连及 HTTP CONNECT 未完成 SSH 握手，SOCKS5 也出现间歇性 banner 超时。
- 密码未写入仓库或报告。检查只执行系统信息、路径存在性和网络连通性探测。

## 实测资源

| 项目 | 结果 |
| --- | --- |
| 内网地址 | `10.9.36.43` |
| OS / 内核 / 架构 | Ubuntu 24.04 LTS / 6.8.0-138-generic / x86_64 |
| CPU / 内存 | 2 vCPU / 3913 MiB |
| 根磁盘 | 19 GiB，总计约 17 GiB 可用 |
| sudo | `sudo -n true` 返回 0 |
| swap | 0 |
| PATH 工具探测 | 未找到 kubeadm、kubelet、kubectl、containerd、ctr、docker、helm |
| 路径探测 | 未发现 `/etc/kubernetes/admin.conf`、`/etc/kubernetes/kubelet.conf`、`/etc/hnb`、`/var/lib/hnb`、`/etc/hnb/bootstrap` |

这些观察仅说明所检查的工具和路径未发现已有安装，不是完整节点所有权认证。

## 目标发现与连通性

候选目标来自仓库既有验收报告，历史记录不能证明目标当前仍存在或仍获授权纳管。

新 worker 于 `2026-10-14T14:38:39Z` 所在 SSH 检查会话输出：

```text
10.9.36.43
default via 10.9.0.1 dev eth0 proto static
10.9.0.0/16 dev eth0 proto kernel scope link src 10.9.36.43
tcp_probe 10.9.127.43 6443 113
tcp_probe 10.9.31.180 6443 113
```

Python `socket.connect_ex` 的 Linux 返回值 113 为 `EHOSTUNREACH`；
本次不能确认原因属于主机释放、路由或安全组，不能据此推断集群已销毁。
两个地址分别来自 Ubuntu/Rocky SSH provider 历史验收报告。

历史平台测试主机 `106.75.34.133` 经 SOCKS5 的只读 SSH 尝试连续在
banner exchange 阶段超时，未能认证或读取当前平台配置。历史报告描述的是
K3s 管理集群，也不能直接假定为本次 kubeadm worker 的目标集群。

## 继续所需输入

- 一个当前可达且用于本次验收的 kubeadm 控制面与其可用访问方式；
- 平台正式 API、已登记目标 ID/当前版本及适用凭据引用；
- node-add 动作资格及运行物料绑定仍需按 6.3 进度记录补齐。

本次未安装软件、未执行 kubeadm init/join/reset、未提交 RuntimeIntent，
也未更改现有集群或本地代理/路由配置。
