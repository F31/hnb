# 3.5 每平面独立身份（真实容器验证版）

- task: 3.5
- requirements: PINST-003, PINST-004, DBND-004
- date: 2026-10-12
- status: done（本轮补上真实 PostgreSQL/NATS 验证，并修正启动与权限缺陷）

## 本轮的实质修正

上一轮 3.5 结论存在三处不可成立之处，本轮以**真实容器**验证并修正：

1. **PG 启动死锁**：原 initContainer 在执行 CREATE ROLE/DATABASE 时，PG 主容器还没启动
   （且它试图等待自己的主容器），集群里根本不会启动。改为官方镜像的
   `/docker-entrypoint-initdb.d` 钩子，在临时 PG 服务启动后、仅新鲜 PGDATA 卷时执行。
2. **PUBLIC 权限**：原脚本只 `REVOKE CONNECT FROM <role>`，没有撤销 PUBLIC。
   任何角色（含未创建的用户）都能连任意平面库。改为
   `REVOKE ALL ON DATABASE %I FROM PUBLIC`，再显式 `GRANT CONNECT, TEMPORARY` 给本平面角色。
3. **NATS 账号从未绑定配置**：原 initContainer 生成账号文件但 nats.conf 不引用。
   改为每个平面一个独立账号，密码来自已存在的 Secret，`$HNB_<PLANE>_PASSWORD`
   环境变量注入 nats-server 配置；账号之间天然隔离（不同账号订阅不交叉）。

同时把「安装时在 Pod 内 `/dev/urandom` 生成密码」改为 **existingSecret**：
生成密码会随 Pod 消失、且无法让用户侧应用读取（在空 Pod 卷里）。
每平面 `existingSecret`（key `password`）在图表 fail-closed 时强制不同，
且禁止与 admin 凭据相同。

## 真实容器验证（本机隔离容器，不是 mock）

### PostgreSQL 16（真实 postgres:16 镜像）

用图表渲染出的 `10-hnb-planes.sh` 原样执行，创建 3 个角色 + 3 个库：

| 场景 | 结果 |
|---|---|
| market 角色连自己的 hnb_market 库 | 成功（current_user=hnb_market） |
| market 角色连 hnb_platform 库 | **拒绝** `permission denied for database` |
| platform 角色连 hnb_market 库 | **拒绝** `permission denied for database` |
| 写入一条业务数据后容器重启 | 数据仍存在（`SELECT 42`） |
| 重启后跨 owner 连接 | 仍被拒（权限不随重启恢复） |

关键点：`REVOKE ALL ... FROM PUBLIC` 生效，`GRANT CONNECT, TEMPORARY` 只给本平面角色。

### NATS 2.11.1（真实 nats:2.11.1-scratch 镜像）

用图表渲染的每平面账号配置（platform/market/artifact 三账号独立密码）：

| 场景 | 结果 |
|---|---|
| platform 账号正确凭据 CONNECT | +OK / PONG |
| market 账号错误凭据 CONNECT | -ERR 'Authorization Violation' |
| market 订阅 `platform.>` 后，platform 发布 `platform.x` | market **未收到**（账号隔离生效） |
| 重启后 /healthz | ok |

## 图表测试更新

`test_candidate_components.py` 的 bundled 测试重写为断言真实行为：

- 无 `initContainers`（PG 不再依赖自等主容器）。
- 包含 `REVOKE ALL ON DATABASE %I FROM PUBLIC` 与 `GRANT CONNECT, TEMPORARY`。
- 不含 `/dev/urandom`（改为 existingSecret）。
- 每平面 Secret 挂载为独立卷、0400 权限。
- NATS 账号引用 `$HNB_<PLANE>_PASSWORD` 且来自对应 existingSecret。
- 13/13 通过。

图表值：`postgresql` repository 改回官方 `postgres:16`、`nats` 改回官方 `nats:2.11.1-scratch`
（digest 由发布锁填充）；新增 `postgresql.securityContext.fsGroup` 与 `nats.tls.existingSecret`。

## 明确限制（不改写成功）

- 用的是本机隔离容器，不是集群 StatefulSet + PV；PVC 持久化与 fsGroup 只在图表
  `helm template` 层面验证，未在集群实际挂载。
- `postgres`/`nats` 官方镜像的 digest 未在此证据里生成（未从 Harbor/公共仓库拉取校验）；
  真实物料身份仍属 3.1/7.1 release lock。
- NATS JetStream 消息级持久化重启验证未完成（scratch 镜像无 shell，未做 stream 数据回读）；
  本证据覆盖账号隔离与进程健康重启。

## 门禁

```bash
python3 deploy/charts/hnb/test_candidate_components.py   # 13/13 OK
helm lint deploy/charts/hnb -f <planes fixture>          # 0 failed
helm template hnb deploy/charts/hnb -f <full fixture>     # 全量渲染通过
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
```
