# 新云主机：hnbctl 与依赖层真机测试

- run_id: `ubuntu-startup-host-test-20261013`
- host: `106.75.34.133`；SSH 用户 ubuntu，免密 sudo
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73`，工作区含未提交变更；二进制摘要另记
- started_at: `2026-10-13T02:34:27.428345+00:00`
- finished_at: `2026-10-13T02:36:18.265121+00:00`（首轮）；修复后 CLI 复测 `02:40:47–02:40:50 UTC`
- exit_code: 0（测试脚本所有断言通过，**不代表整平台安装完成**）
- environment: Ubuntu 24.04 LTS / kernel 6.8.0-138-generic / x86_64 / cgroup v2 / 无 swap
- result: CLI 冒烟与拒绝路径通过；依赖重启/备份恢复通过；整平台部署 blocked

## 命令与制品身份

本机编译后上传，减少云主机编译缓存与资源占用：

```bash
# cmd/hnbctl 目录
go build -trimpath -o /tmp/opencode/hnbctl-cloud-test .
# 仓库根目录；SSHPASS 由调用环境提供，不写入报告/脚本。
python3 docs/acceptance/reports/20261013-ubuntu-startup-host-test.py \
  --host 106.75.34.133 --binary /tmp/opencode/hnbctl-cloud-test --reboot
```

原始 CLI SHA-256：`66ecac8cdb740ef074c3c9fd51fa1e372f70021ff6429a67eab76022d998ef39`。
修复后 CLI SHA-256：`8bc1f569a0593b358bab3052e690deb3372cd03cf54363841e8becd5994df23c`。
每次上传均核对远端二进制摘要。原始运行 JSON、stdout/stderr、复测输出、
fixture manifest、节点输入和脚本摘要见同名前缀文件及 `-artifacts.sha256`。

SSH 曾发生握手中断。本机清除该连接进程的 127.0.0.1:7892 代理环境变量后，
目标路由仍经虚拟网卡 eth5/198.18.0.2；加入目标专用路由
`106.75.34.133/32 via 192.168.43.1 dev eth2`，使用 curve25519-sha256 后稳定。
未全局关闭代理，也没有修改云主机的代理配置。

## 真实资源与利用情况

| 项目 | 测试前 | 测试后 |
|---|---:|---:|
| CPU | 2 vCPU | 2 vCPU |
| MemTotal | 3913Mi | 3913Mi |
| MemAvailable | 3524Mi | 3452Mi |
| 根文件系统总量 | 19,682,557,952 bytes ≈ 18.33GiB | 同左 |
| 根文件系统可用量 | 17,475,899,392 bytes ≈ 16.28GiB | 17,101,299,712 bytes ≈ 15.93GiB |

云盘标称 20GiB，分区/文件系统实际容量更小。容量输入保守向下取整为 18Gi，
没有把主机声明成 40Gi。联网安装 distro PG/NATS、测试数据、CLI、备份合计
增加磁盘使用约 357MiB，保留了约 16GiB 可用空间。
NATS 单进程 RSS 快照 14,452KiB；PG 有多个进程与共享页，未把 RSS 求和当作
独占物理内存。上述是小样本实验快照，不是平台满载或长期增长预算。

## hnbctl 真机结果

| 命令/断言 | 返回码 | 结果 |
|---|---:|---|
| `version` / `help` / `matrix` | 0 | 二进制在实际 Ubuntu 主机正常执行，候选版本与资格范围可见 |
| `init --mode bootstrap --profile Minimal --os ubuntu-24.04` | 0 | 创建配置草稿 |
| `validate` / `preflight` / `install` 使用不完整草稿 | 2 | 正确拒绝缺少入口、TLS、存储、节点绑定的配置；仅测拒绝路径 |
| `capacity`：真实 2C/3913Mi/18Gi + Startup fixture | 3 | `DECLARED_CAPACITY_INSUFFICIENT`，磁盘规划不足 |
| `capacity`：明确标识的假设 40Gi 正向对照 | 0 | 规划通过，仍 `installAuthorized=false`；不是实际磁盘观测 |
| `verify` 未绑定部署观测源 | 4 | `PLATFORM_NOT_VERIFIED`，未伪报平台成功 |

Startup fixture 使用合成镜像 digest、Secret 名称与测试 DSN，只做渲染/容量测试，
没有将它作为真实制品部署到主机。真实 resources 为 1075m/1792Mi/15Gi PVC；
startup scope + 外部缓存/备份假设下 required 为 1683m/2996Mi/27Gi。
CPU 与内存通过，实际根盘不足；外部缓存/备份参数本身不配置外部存储。

### 测试发现与修复

首轮 `verify --level node` 忽略层级选择，并把“目录中缺失的检查数量”误当作
“未验证检查数量”，提示 0 项未验证。修复位于：

- `cmd/hnbctl/internal/cli/verify.go`：按层级选择检查和显示结果；统计非 passed 项。
- `cmd/hnbctl/internal/cli/verify_test.go`：四层级 JSON/human 过滤与计数回归。

修复后重新编译上传，在同一云主机分别测试 node/cluster/platform/managed：
各层检查数量正确为 **2/3/8/2**，均继续退出 4；delivery verdict 保留全层资格，
不因单层检查而宣布完整交付。原始缺陷输出保留，未覆写为修复后的结果。
本机 `go test -race ./... -count=1`、`go vet ./...` 和重新编译均通过。

## 依赖层真机断言

1. 联网安装 PostgreSQL `16.15-0ubuntu0.24.04.1`、NATS `2.10.7-1ubuntu0.3`。
2. 三个独立 owner 测试库各写入一条记录；JetStream 文件流 HNB_DRILL 写入一条消息。
3. 合成 identity/KMS 32-byte 文件权限 0600；不是正式 HNB 身份系统验收。
4. 三库 `pg_dump -Fc` + 停服复制 JetStream store + 合成材料备份；9 文件 SHA-256 验证。
5. 真实主机 reboot：boot ID 从 `89e7b556-cbc0-4b59-a453-5b9bcc7f0935`
   变为 `2a752711-13cf-42e3-9bf5-aa904ef67e04`。重连后数据库、消息、材料摘要全部一致。
6. `pg_restore` 到新的 `_restored` 库、恢复 quiesced JetStream store 和材料；
   恢复+核验 **0.797s**，非生产 RTO/RPO 声明。
7. platform owner 读 market 测试表返回 `permission denied`。
8. PG 只监听 127.0.0.1:5432，实验 NATS 只监听 127.0.0.1:14222，未公网暴露测试依赖。

## 主机留存与部署范围

主机保留 `/home/ubuntu/hnb-startup-host-test-20261013/` 下 CLI、草稿和 fixture，
以及 `/var/lib/hnb-cloud-drill-20261013/`、三个测试库、三个恢复库、
`hnb-cloud-drill-nats.service`。NATS 实验 store 上限为 64MB memory / 256MB file。
未安装 Kubernetes/containerd，没有 Core Pod、平台登录或真实 Operation 验收。

整平台阻塞项：当前 Startup 默认磁盘预算超过根盘；实际 release lock、镜像
digest 与身份/TLS/Secret bindings 尚未交付；Startup 组合尚未接入 hnbctl 完整
安装验收。**CLI 拒绝路径或依赖恢复通过，不提升整平台/离线/HA 资格**。
继续利用现有主机应逐项补齐制品与入口，并以小数据预算实测新的部署组合，
不直接删除系统预留或把测试 fixture 当发布包。
