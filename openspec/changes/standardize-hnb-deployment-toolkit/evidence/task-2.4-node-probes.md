# 2.4 只读节点探测

- task: 2.4
- requirements: DPLY-002, PINST-001
- date: 2026-10-11
- status: done（节点探测方法范围）

## 实现

`cmd/hnbctl/internal/toolkit/node_probe.go` 提供固定观察程序与 SSH adapter：

- 读取 OS/version、内核、架构、cgroup、UID、CPU、MemTotal、根文件系统可用空间、swap 及已知 owner 标记。
- 观察程序不安装软件、不修改策略、不加载镜像、不创建资源、不运行 reset/init/join，不读取 kubeconfig 内容。
- SSH 使用 existing known_hosts、StrictHostKeyChecking=yes、UpdateHostKeys=no、BatchMode、固定脚本；非 root 通过现有 `sudo -n` 执行。
- 输出最大 16 KiB，严格拒绝未知/缺失/重复字段、异常数字与任意日志；错误报告不回显 SSH stderr/凭据。
- 单节点 30s timeout，稳定 checkId/reasonCode/observedAt/expected/actual/remediation。
- OS/kernel 仅放行实际已有报告中的精确候选组合；Ubuntu 多节点缺独立证据时阻塞。
- CLI 使用 `ProbeLockedNodes` 将实际 OS/arch 绑定至所选 release lock；方法级候选不能悄悄使用另一 OS/arch 的物料配方。
- 节点最低资源检查为 2 CPU/3 GiB RAM/10 GiB root free，仅是节点准备门，不是完整平台容量认证。
- 已知 Kubernetes/HNB/RKE2/K3s 标记阻塞。未发现标记不等于已完成全面 ownership 证明；4.4 边界仍需独立验证。

CLI `preflight -f ... [--lock ...] [--output report.json]` 先做结构、凭据、锁和候选兼容检查。
bootstrap 路线随后运行只读节点观察；existing 路线的集群检查集尚未实现。
报告总是包含 `PREFLIGHT_CHECK_SET_INCOMPLETE`，加上实际 profile/route/offline blocked 项，不能授权安装。
本地 JSON 报告按请求新建为 0600，拒绝覆盖已有文件。2.5/2.6 仍未勾选。

## 自动测试

`node_probe_test.go` 验证 9 项候选检查、11 组失败观察、畸形/超大/缺失/泄密响应拒绝、
输入无效不联系节点、Ubuntu 多节点不推导认证；实际运行 shell 到隔离 fixture，比较全部文件内容/权限和目录项不变。
`TestObservedPlatformMustMatchSelectedLock` 验证实际 OS/arch 和配方不匹配时 blocked，缺锁时不联系节点。
初次 fixture 测试暴露本地 stat 的 `ext2/ext3` 输出，被过严 token 检查拒绝；修正 cgroup token 支持该实际文件系统名后通过。
这不放行 ext2/ext3，只有 cgroup2fs 才通过 cgroup 检查。

`TestPreflightPartialReportCannotAuthorizeInstallation` 验证输出 blocked、配置/锁身份、报告 0600、无凭据内容、输入不变与拒绝覆盖。

## 真机测试（用户已重启 Ubuntu 节点）

环境：`106.75.26.57`，节点 `hnb-ubuntu-cp`，内网 `10.9.127.43`，用户 `ubuntu` + existing sudo，已有 Kubernetes。
前置操作：本地 known_hosts 未有此主机，测试 setup 单独通过 accept-new 注册首次观察到的 ED25519 主机密钥并执行 `true`。
实际 probe/snapshot 均使用 strict existing host-key trust，不在 probe 内修改 known_hosts；无远端初始化动作。

实际命令（工作目录 `cmd/hnbctl`；路径仅指向既有私钥，不记录其内容）：

```bash
HNB_NODE_PROBE_E2E_SPEC='{"name":"hnb-ubuntu-cp","roles":["control-plane"],"sshAddress":"106.75.26.57","internalAddress":"10.9.127.43","user":"ubuntu","sshKeyFile":"/tmp/hnb_provision_key"}' \
go test -race ./internal/toolkit -run '^TestNodeProbeRealSSH$' -count=1 -v -timeout=2m
```

退出 0，测试用时 2.37s，race 包总用时 3.395s：

| checkId | actual | verdict |
|---|---|---|
| node.os-kernel | ubuntu-24.04 / 6.8.0-31-generic | pass |
| node.arch | x86_64 | pass |
| node.cgroup | cgroup2fs | pass |
| node.permission | uid 0（ubuntu 经现有 sudo） | pass |
| node.cpu | 2 | pass |
| node.memory | 4007932 KiB | pass |
| node.disk | 14831992 KiB available | pass |
| node.swap | 0 entries | pass |
| node.owner-markers | kubernetes | blocked / NODE_OWNER_PRESENT |

前后检查 `/etc/fstab`、sysctl/containerd/kubelet 配置摘要、owner 标记 metadata 和已安装 package inventory，逐字节一致：

- snapshot before=after：`sha256:3b033958195e8ff3440520c92f6ae1c8efa8079d42d9520a7fc427017e4c81d4`
- fixed probe script：`sha256:850ed42f7060ae478e5fa30eef4a4e23833f06b56cd8e89b5af3553a2fcf8663`

结果证明：该已初始化节点被拒绝 bootstrap，观察未改变所核验配置/软件状态。
不证明重启后的 Kubernetes/Cilium/API Ready、PG/消息持久化或完整平台可用；这些仍需各自验收。

## 保留失败与环境变更

先尝试 Rocky `106.75.8.151` 的只读测试：before snapshot 阶段 SSH 失败，测试退出 1（10.03s）；
独立诊断为 TCP22 connection timed out。用户随后确认该机器已删除；不再重试，不将此节点列为当前可用环境。
已有 Rocky 历史 provider 报告保持其原有认证范围；Ubuntu 新测试不替代 Rocky 本轮未执行的观察验收。

## 最终门禁

本轮最终 `go test ./... -count=1`、`go vet ./...`、`go test -race ./... -count=1`、
`go build -o /tmp/hnbctl .` 均退出 0；普通测试未设置真机环境变量，显式真机测试的独立结果见上文。
`openspec validate --all --strict`：29 passed / 0 failed；
`node scripts/validate-openspec.mjs`：31 specs / 242 requirements / 377 scenarios；`git diff --check` 通过。
