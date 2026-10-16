# Startup 组合在 2C4G 真机的负载与容量实测

- run_id: `ubuntu-startup-capacity-20261013`
- host: `106.75.34.133`；Ubuntu 24.04；2 vCPU / 3914 MiB；根盘 18.33 GiB；单节点 K3s v1.36.5+k3s1
- started_at: `2026-10-13T12:52:28.611781+00:00`；finished_at: `2026-10-13T12:55:21.959793+00:00`
  （以统计 JSON 的真实记录为准；此前正文手写的 `12:48:33` 无对应数据，已更正）
- base_commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73`（另有未提交工作区改动，仅提交号不足以标识源码）
- artifact_digests:
  - `20261013-startup-capacity-under-load.json`:
    `sha256:c433630c63c636d4e4e42dad4ca33984631e27bd405c6d56cc0a69cfc2800d02`
  - `20261013-startup-capacity-under-load.py`:
    `sha256:8d5f75d9be050e71840ec72a305fce8abfaad9541d19fccadbb61dc2b3dd4ca7`
- exit_code: `0`
- 结论：**在本次只读门户/登录负载下，Startup 10 工作负载保持运行；吞吐约 118–150 ops/s，
  并发升高时延迟上升明显，内存始终充足。** 受下文方法限制，这些数字是带负载条件的
  测量，不构成生产容量认证，也不足以判定 PG 是唯一瓶颈。

## 方法与范围

- 从宿主机经 portal Service ClusterIP 发起并发请求，每个 worker 循环执行
  `POST /api/v1/auth/login`（admin 账户）→ `GET /api/v1/workspaces` →
  `GET /api/v1/tenants`，统计每类操作的分位数延迟、吞吐与错误分类。
- cgroup v2（`kubepods.slice`）按容器读取 CPU `usage_usec` 与 `memory.current`，
  按 Pod 汇总；同时采样宿主机可用内存、loadavg、apiserver `/livez`、`/readyz`
  以及 PostgreSQL `pg_stat_activity` 连接行数。
- 场景：idle 15s 后依次 1/2/4/8/16 个并发 worker，各 30s。
- **已知方法限制（影响结论强度，必须与结果一并阅读）**：
  - CPU 以固定 2s 归一化，但采样循环还要遍历容器、调用 `sudo` 与子进程并 sleep，
    实际间隔大于 2s，因此"核当量"是近似值，偏低。
  - 各组件峰值来自不同采样时刻，**不可相加**；不能据此宣称某一时刻的合计用量。
  - 分位数仅统计成功请求；429 按循环异常单独计数，不是总请求错误率。
  - 每档 elapsed 固定 30.5s，线程 join 与收尾探针耗时未计入吞吐分母。
  - 负载生成器与被测服务同机运行，会争用同一批 vCPU。
  - 只连续探测了 apiserver 的 `/livez`/`/readyz`；脚本未保留全部 Pod 的重启前后
    对比，因此"全程 10 Running 无重启"的表述证据不足，仅能说测试结束后 10/10 Running。
  - `pg_stat_activity` 计数包含 idle 与系统会话，**不是活跃连接数**，也未核对
    `max_connections` 是否会被耗尽。
- 仅覆盖已部署的 Startup 初始组合与登录/门户读取工作负载，**未校验**：
  生产容量认证、写密集/上传/下载/真实 Operation、Agent/Read Model、HA、离线。
  本报告是带负载条件的测量，不是发布 SLA。

## 延迟与吞吐

| 场景 | ops/s | 错误 | 登录 p50 | 登录 p99 | workspaces p50 | readyz/livez | PG 连接 min/max/avg |
|---|---|---|---|---|---|---|---|
| idle_0w | 0 | 0 | – | – | – | 204/204 | 14/14/14 |
| load_1w | 144.5 | 69×HTTP429 | 4.3ms | 17.9ms | 2.5ms | 204/204 | 14/14/14 |
| load_2w | 147.0 | 84×HTTP429 | 8.3ms | 48.0ms | 4.7ms | 204/204 | 14/15/14.6 |
| load_4w | 149.4 | 76×HTTP429 | 23.7ms | 82.6ms | 12.9ms | 204/204 | 14/17/15.7 |
| load_8w | 130.0 | 0 | 72.8ms | 192.4ms | 45.4ms | 204/204 | 16/21/19.2 |
| load_16w | 117.8 | 0 | 181.7ms | 384.8ms | 113.5ms | 204/204 | 14/27/22.2 |

- 吞吐在 2 台并发 worker 时即触顶约 147–150 ops/s，之后不再随并发增长。在本测量
  条件下，apiserver 与 PostgreSQL 是 CPU 占用最高的两项，但**未做对照实验**，
  不能据此断定 PG 是唯一瓶颈。
- 1w/2w 出现的零散 HTTP 429 与单 worker 高频连续登录同时出现；未逐条核对具体命中的
  路由与其限流配置（路由侧全局 `rate_limit(100/s)`），因此只记录现象，不下因果结论。
  8w/16w 档 429 归零同样只是观察结果。
- 延迟随并发明显劣化：8 并发登录 p99 约 192ms，16 并发约 385ms。这是**并发生成器
  线程**下的读负载延迟，不能直接换算为"8 个真实用户"的生产交互上限。

## 资源实测（峰值）

| 工作负载 | 峰值 CPU（核当量） | 峰值内存 |
|---|---|---|
| apiserver | 0.459 | 26.2 MiB |
| app-market | 0.002 | 15.1 MiB |
| cluster-router | 0.001 | 12.1 MiB |
| extension-controller | 0.001 | 14.1 MiB |
| NATS | 0.001 | 19.6 MiB |
| operation-worker | 0.002 | 15.1 MiB |
| platform-api | 0.002 | 14.4 MiB |
| portal | 0.057 | 17.8 MiB |
| postgresql | 1.145 | 281.6 MiB |
| tunnel-server | 0.002 | 11.6 MiB |

- 峰值观测：负载期间宿主机可用内存最低约 2738 MiB（总量 3914 MiB），内存不缺；
  loadavg 峰值 11.29（2 vCPU，含同机负载生成器与 K3s 外围）。各服务峰值 CPU 见上表，
  但峰值发生在不同采样时刻，**不能相加**为某一时刻的服务合计占用。
- apiserver 的 `/livez` 与 `/readyz` 在各档采样中均返回 204；测试结束后 10/10 Running。
  脚本未保留全部 Pod 的重启计数前后对比，故不宣称"全程无重启"。
- PostgreSQL 的 `pg_stat_activity` 行数从 14 升到最多 27。该计数含 idle 与系统会话，
  未核对 `max_connections`，因此只能说明**未观察到连接耗尽迹象**，不能声称连接池余量。
- PostgreSQL 在本次负载中 CPU 占用最高（约 1.15 核当量，近似值），apiserver 次之
  （约 0.46 核当量），其余服务均远低于 0.01；与"请求落库集中在 PG"的架构判断方向一致。

## 对 2C4G 边界的意义

- 内存充足（峰值总用约 0.4 GiB，可用保持 2.7 GiB），Startup 初始组合在
  2C4G 上留有余量；磁盘占用 5.5 GiB 受数据量而非负载影响。
- 在本次**只读**登录/门户负载下，CPU 先于内存成为约束；要把这一观察升级为容量结论，
  需要补充对照实验（分离负载生成器）、真实用户模型的并发定义、全部 Pod 的重启与
  错误证据，以及写密集与真实 Operation 负载。
- 观测到的单组件峰值 CPU 可用于校准 requests（见 3.8 容量模型的负载校准输入），
  但因采样间隔偏差与峰值不同步，只能作为量级参考，不能作为发布认证。

## 复现与证据

- `20261013-startup-capacity-under-load.py`：负载与采样脚本（需宿主 sudo + k3s kubectl）。
- `20261013-startup-capacity-under-load.json`：完整场景、Pod、宿主机聚合数据。
- 命令：`sudo -n python3 /home/ubuntu/capacity-test.py > /home/ubuntu/capacity-test.log 2>&1`。