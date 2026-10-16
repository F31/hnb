# 3.5 PG/NATS bundled/external 与每平面独立身份

- task: 3.5
- requirements: PINST-003, PINST-004, DBND-004
- date: 2026-10-11
- status: done（配置契约、Chart 与渲染证据）；真实跨 owner 拒绝未在集群执行

## 每平面独立身份（配置契约）

`cmd/hnbctl/internal/toolkit/config.go` 新增 `infrastructure.planes`，
声明三个 owned plane：`platform`、`market`、`artifact`。每个 plane 必须声明
自己的 `database`、`role`、`postgresqlSecretRef`、`natsSecretRef`。

`validatePlanes()` 拒绝：

- 缺少任一 owned plane，或出现非 owned plane，或重复声明；
- 两个 plane 共用同一个 postgresql / nats 凭据、role 或 database；
- 缺失任一 Secret 引用，或引用里内联凭据；
- database / role 标识不符合 PostgreSQL 标识符规则。

**为什么强制不共用身份**：若各 plane 共用一个账号，market 就能读写 platform 的表，
所谓「跨 owner 访问被拒绝」在下层根本不存在，只能靠应用代码自觉。

过程中修正了两处我自己的错误判断：

1. 最初用 `validName`（Kubernetes DNS 规则）校验 database/role，会拒绝
   `hnb_platform` 这类含下划线的合法标识。新增 `validDatabaseIdent`
   （`^[a-z_][a-z0-9_$]*$`、≤63 字节，对齐 PostgreSQL 而非 Kubernetes 的 253）。
2. 最初禁止同一 plane 内 database 与 role 同名。PostgreSQL 中这是常见做法，
   安全边界是**跨 plane** 隔离，而非 plane 内命名不同。该规则已移除。

## 凭据不入 values

新增 `cmd/hnbctl/internal/toolkit/infra_plan.go`：

- `BuildInfraPlan` 把配置解析成只含引用的 `InfraPlan`；配置不安全即报错。
- `RenderInfraPlan` 输出运维可读文本前**再次**按形状扫描
  （DSN 带口令、PEM 私钥、长 base64），命中则拒绝输出。

Chart 侧同样收紧：

- 新增 bundled `postgresql` / `nats` 子 Chart，数据库密码与 NATS 账号在安装时
  由 init container 用 `/dev/urandom` 逐 plane 生成到内存卷，**不经过 values**。
- PostgreSQL admin 密码必须来自 operator 提供的 existingSecret（`adminSecret`），
  无默认值；缺失即拒绝渲染。
- NATS 账号文件由 init container 生成后只读挂载；ConfigMap 里只有端口与
  JetStream `store_dir`，不含任何口令。
- **删除 `templates/secrets.yaml`**：它原本创建一个内容为
  `"change-me-tunnel-secret"` 的 Secret。这是把一个已知共享密钥装进每一个
  没覆盖该值的安装，且会进入 release values 元数据。改为
  `global.existingSecretName` 引用既有 Secret；`_component.tpl` 同步引用该名称。

## 文件持久化与跨 owner 拒绝

`postgresql`：StatefulSet + PVC（`storage.className`、可复用 existingClaim）。
init container 对每个 plane 执行 `CREATE ROLE`（口令取自 `/dev/urandom` 生成的
内存文件）与 `CREATE DATABASE OWNER`，并对**其它** plane 的数据库执行
`REVOKE CONNECT ON DATABASE ... FROM <role>`——跨 owner 访问在数据库层被拒绝，
而不是留给应用代码。

`nats`：StatefulSet + PVC，JetStream `store_dir` 指向该 PVC。

两者的 bundled / external 切换由 `postgresql.enabled` / `nats.enabled` 控制；
关闭后渲染结果中 StatefulSet 与 PVC 均为 0（已实测）。

## 测试

`cmd/hnbctl/internal/toolkit/infra_plan_test.go`（Go）：

- `TestPlaneIdentitiesMustBeDistinct`：10 组场景——共用 pg 凭据、共用 nats 凭据、
  共用 role、共用 database、缺 plane、非 owned plane、重复 plane、缺 pg 引用、
  缺 nats 引用、在引用里内联 DSN；每组都要求 `Validate` 报错**且**
  `BuildInfraPlan` 拒绝出计划。
- `TestDatabaseIdentifierRules`：合法/非法标识符，及注入形态被拒。
- `TestInfraPlanCarriesReferencesNotCredentials`：渲染结果只含引用，
  不含 `hunter2`/`password`/`postgres://`/PEM。
- `TestRenderInfraPlanRefusesPollutedInput`：即使外部传入被污染的计划，渲染器也拒绝。

`deploy/charts/hnb/test_candidate_components.py` 新增 4 项（共 13 项）：

- `test_postgresql_persists_and_isolates_planes`：PVC + storageClass、
  每 plane 的 `CREATE ROLE`/`CREATE DATABASE`、跨 plane `REVOKE CONNECT`、
  口令经 `/dev/urandom` 生成、admin 与 POSTGRES 口令均为 existingSecret，
  且所有 `*PASSWORD` 变量都走 `valueFrom`。
- `test_nats_jetstream_persists_to_disk`：PVC、ConfigMap 含 jetstream/store_dir、
  逐 plane 账号生成、ConfigMap 内无 password。
- `test_no_credential_material_in_rendered_output`：渲染输出正则扫描。
- `test_bundled_infrastructure_requires_its_secret_refs`。

## 过程中由测试抓出的真实缺陷

- 两个 bundled 子 Chart 最初直接写 `{{ .Values.image.repository }}@{{ .Values.image.digest }}`，
  **绕过了共享 `hnb.image` helper**，导致 `global.imageRegistry` 对它们无效。
  由 `test_global_registry_retargets_all_images` 抓到并改为统一走 helper。
- 首版 init container 用 Helm `range` 在 shell 里展开 plane 列表，产生
  `undefined variable "$plane"` 作用域错误；改为通过 `HNB_PLANES` 环境变量传入，
  模板不再嵌套展开。

## 验证（实际执行，全部退出 0）

```bash
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY no_proxy='*'
helm lint deploy/charts/hnb
helm template hnb deploy/charts/hnb -f <全组件 digest + 依赖 fixture>
python3 deploy/charts/hnb/test_candidate_components.py        # 13 tests OK
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
go vet ./cmd/runtime-target-lifecycle-provider/...
go test -race ./cmd/runtime-target-lifecycle-provider/... -count=1
openspec validate --all --strict ; node scripts/validate-openspec.mjs
```

全量渲染：28 个镜像引用全部 digest 固定（非 digest 0），2 StatefulSet + 2 PVC，
凭据形状扫描 0 命中。

## 未完成项与范围限制

- **未在集群中运行**：跨 owner 访问被拒绝目前只有模板级证据（`REVOKE CONNECT`
  出现在渲染结果中），未实际连接数据库验证 market 角色确实连不上 platform 库。
  这是本任务最主要的未验证部分。
- 未验证 Minimal 的重启后持久化（PVC 重挂载后数据仍在），只验证了 PVC 被声明。
- bundled 组件未构建镜像，`image.digest` 只能填真实发布摘要后才可安装。
- 渲染输出中 `db.dsn` fixture 为 `postgres://fixture/fixture`（无口令），
  测试正则覆盖带口令形态；生产配置不应在 values 中出现 DSN。
- **超出本任务但已发现**：`templates/rbac.yaml` 给平台 ServiceAccount 授予
  `*/*/*` 集群级权限，与最小权限原则冲突。本轮未改动（不属 3.5 范围），
  建议单独处理。
