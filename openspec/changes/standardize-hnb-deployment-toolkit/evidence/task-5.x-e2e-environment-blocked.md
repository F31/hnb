# 5.3/5.4/5.5 环境阻塞记录

- tasks: 5.3, 5.4, 5.5
- requirements: PINST-002, PINST-005, DBND-005, PINST-001, DPLY-003, DBND-002
- date: 2026-10-12
- status: **blocked（环境不可用）**；三项均保持未勾选

## 阻塞原因

本轮开始时对 5.3 所需的真实环境做了连通性核查，结果为**环境当前不可用**：

```text
# TCP 层
exec 3<>/dev/tcp/106.75.26.57/22   -> SSH_22_REACHABLE
exec 3<>/dev/tcp/127.0.0.1/80     -> HARBOR_LOCAL_REACHABLE
/tmp/hnb_provision_key            -> 存在 (0600)

# SSH 层
ssh -i /tmp/hnb_provision_key -o StrictHostKeyChecking=yes -o BatchMode=yes \
    ubuntu@106.75.26.57 'echo ok'
  -> kex_exchange_identification: Connection closed by remote host

# 读取 SSH banner（两次重试）
exec 3<>/dev/tcp/106.75.26.57/22; head -c 100 <&3
  -> 空
```

TCP 22 端口可建立连接，但**从未返回 SSH banner**，随后远端主动关闭。这不是
认证失败、host key 不匹配或 known_hosts 问题（那些会在 banner 之后才发生），
而是 sshd 侧无响应：可能是主机资源耗尽、sshd 进程挂起，或中间网络设备接受
连接但不转发到 sshd。

因此无法在该节点执行任何 E2E：读不到 `kubectl`、读不到集群状态、无法观察
CNI/平台组件，也无法采集 5.3 要求的完整业务闭环证据。

## 未做的事（明确保留）

- 未用已有节点探针/fixture/local TLS 测试冒充 5.3 的真实平台 E2E。
- 未把历史上的 Ubuntu 验收记录重新标记为"当前首次成功"。此前 2.4/3.2/Harbor
  收敛的真机记录属于**节点与 CNI 层**证据，不等于 Minimal 平台完整业务闭环。
- 5.4 要求的"冷缓存 bootstrap/恢复"与 5.5 要求的"Rocky 独立环境"同样无环境，
  保持 blocked。

## 影响的下游任务

4.4（专用空白管理基座 bootstrap 适配）的前置条件本就依赖 5.3/5.4/5.5 的完整
平台证据，已在 `task-4.4-blocked-boundary-contract.md` 记录为阻塞。5.3 不可执行
意味着该阻塞不解。

## 解锁条件

需要一个可正常提供 SSH banner 且具备完整平台预算的独立环境（5.3 已有
Kubernetes；5.4 空 OS Ubuntu；5.5 Rocky），并已确认资源预算满足 3.8 容量估算。
环境恢复后按 `evidence/task-3.8-capacity.md` 的预算核对，再执行对应 E2E。
