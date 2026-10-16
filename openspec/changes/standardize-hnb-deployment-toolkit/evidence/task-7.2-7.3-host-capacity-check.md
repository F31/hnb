# 新云主机 106.75.10.155 资源核查（7.2/7.3 前置）

- task: 7.2/7.3 前置核查
- requirements: DBND-002, DBND-003, DBND-005
- date: 2026-10-12
- status: blocked（主机可用，但资源不满足 7.2 Minimal 平台预算）

## 连通性与主机状态

用户新建云主机 `106.75.10.155`（Ubuntu，账号 ubuntu）。本轮完成连通性恢复与
只读环境核查：

```text
# 本工作区默认路由经 eth5/198.18.0.2，SSH 握手间歇失败。
# 目标专用直连路由后连接稳定，不能据此确定具体由 7892 进程拦截。
ip route add 106.75.10.155/32 via 192.168.124.1 dev eth2   # 直连路由

# 实测主机信息（全部否定了"高配或满足预算"的假设）
Ubuntu 24.04 LTS / kernel 6.8.0-31-generic / x86_64
2 vCPU, 3.8 GiB 内存（3891 Mi）, 19 GiB 磁盘（已用 1.6G）
cgroup v2，passwordless sudo，无 swap，无 containerd/docker/kubeadm
```

## 容量判定（不可通过）

用真实 `helm template`（Minimal overlay + 完整组件 values）+ `hnbctl capacity`
给出的预算（见 `task-3.8-capacity.md`）。**本轮已校准 Chart 资源**（权威来源为
各 subchart values.yaml）：Minimal requests 从 4600m/6016Mi 降至 **1900m/2752Mi**，
核心服务 150m/256Mi、常规 100m/128Mi、低频控制器 50m/64Mi，并为容量模型加入
外部缓存/备份承载、每节点系统预留覆盖与 `--scope startup` 规划档。

| 项 | 7.2 Minimal 单节点最小可计划值（startup） | 新主机实际 | 判定 |
|---|---|---|---|
| CPU | ≥2590m（requests 1900m×1.1 + 预留 500m）→ 实际需 ~3 核节点 | 2000m | 不足 |
| 内存 | ≥4052 Mi（requests 2752Mi×1.1 + 预留 1024Mi）→ 实际需 ~6Gi 节点 | 3891 Mi | 不足 |
| 磁盘 | 外部缓存/备份+精简预留 ≥43Gi → 实际需 ~50Gi 节点 | 19 Gi | 不足 |

即使以 `--cache-source external --backup-source external --scope startup` 规划，
本机仍返回 `DECLARED_CAPACITY_INSUFFICIENT`。**结论：校准后的完整 19 服务
Minimal 在 startup 档需要 3C/6Gi/50G 节点；2C4G 差 CPU 约 590m。降到 2C4G
需要服务合并（低频控制器并入 extension-controller），不是单纯调 requests。**

7.2 要求安装过程禁止出站公网；公网 SSH 管理地址本身不构成阻塞。未来可保留管理
访问并限制出站流量，结合实际连接观测确认无公网 fallback；仅禁 DNS 不足以证明。
本轮未设置出站限制。

## 结论与解锁条件

保持 7.2/7.3 未勾选（blocked）。解锁需要一台资源满足单节点 Minimal 最小可计划值
（**约 3c/6Gi/50G**，startup 档 + 外部缓存/备份承载）且可配置为禁出站公网的
Ubuntu（7.2）与 Rocky（7.3）环境。已记录新主机可达性方案，供后续环境复用时
直接使用。

## 后续真机利用

该 2C4G 主机已完成[依赖层节点重启与备份恢复实验](../../../../docs/acceptance/reports/20261012-ubuntu-dependency-drill.md)，
包括 PostgreSQL/JetStream/合成身份材料；它们可在当前规格执行。完整平台预算阻塞
仅影响完整安装认证，不影响这些范围明确的依赖层实验。
