# 3.8 容量规划的真机负载校准（Startup / 2C4G）

- task: 3.8（校准输入）、5.x（带负载条件的容量证据）
- requirements: DBND-004, PINST-002
- date: 2026-10-13
- status: done（实测观测）；Planning 数值仍是建议值，未转为认证
- 报告：`docs/acceptance/reports/20261013-startup-capacity-under-load.md`
- 数据：`docs/acceptance/reports/20261013-startup-capacity-under-load.json`
- 脚本：`docs/acceptance/reports/20261013-startup-capacity-under-load.py`

## 为什么需要这次实测

3.8 的 requests/存储预算来自 Chart 声明值 + 系统预留与余量推算，属于**规划**。
本次在 `106.75.34.133`（2 vCPU / 3914 MiB）上对已部署的 Startup 10 工作负载施加
登录 + 工作空间/租户读取的并发负载，得到真实的每 Pod CPU/内存、延迟分布与
PostgreSQL 连接数，用于校准容量模型并明确 2C4G 的边界。

## 关键实测结论

- **吞吐触顶而非线性扩展**：登录+两次读取的组合在 2 并发 worker 即达约 147–150 ops/s，
  之后并发增加只抬高延迟（8w 登录 p99 约 192ms；16w 约 385ms），总量反而回落到 118 ops/s。
- **CPU 是瓶颈**：峰值服务侧合计约 1.7 核（postgresql 1.145 + apiserver 0.459 + portal
  0.057），postgresql 是主要 CPU 开销；2 vCPU 无法在更高并发下继续扩容。
- **内存不紧张**：负载期间宿主机可用内存最低约 2738 MiB（总量 3914 MiB）；Pod 峰值内存
  合计约 0.4 GiB（postgresql 282MiB 为最大单项）。
- **探针与依赖稳健**：所有场景中 apiserver `/livez`、`/readyz` 均 204；PostgreSQL 活动
  连接峰值 27，未耗尽；无 Pod 重启。
- **1w/2w/4w 出现零星 HTTP 429**：单 worker 高频连续登录触发限流，耦合到 8w/16w 后归零，
  属正常保护行为而非崩溃。

## 对规划模型的用法

- 单组件峰值观测（apiserver ≈ 0.46 核、portal ≈ 0.06 核、各辅助服务 < 0.01 核）可作为
  requests 校准的输入下限，但**不替代**认证；真实生产还有并发 Operation、Agent、
  上传/下载等本轮未覆盖的负载。
- 2C4G 的实测上限约支持 8 个并发交互门户用户；更高吞吐需要 apiserver/PostgreSQL 扩容
  或外置更大数据库。Minimal/HA 档位容量仍按 3.8 规划值并保持 blocked。

## 范围与不确定性

- 负载由宿主同机生成，与被测服务共享 2 vCPU，绝对值偏悲观、仅作相对比较；
- 仅测登录/读取，不含写密集、上传、真实 Operation/Agent；
- 未观察内存峰值以外的极端情形（大数据量、长时间）。结论为 Startup 初始组合在 2C4G
  的**带负载观测**，不是生产容量认证。