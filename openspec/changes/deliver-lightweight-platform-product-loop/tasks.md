## 1. S0 / P0 — 范围、契约与可测量基线

进入条件：本提案获批。各任务建议约一工作日；发现范围过大时先拆增量再实施。所有任务初始未完成；完成须附实际命令、源码/物料身份、报告及 Requirement 映射，缺环境或 owner 记 blocked，不用较低层验证代替。

- [x] 1.1 盘点已有 registry、实际实现及两个依赖 change 的任务/Requirement，产出“复用/缺口/owner/证据”表；核对已勾选任务与业务验收范围，禁止重复实现或自动提升认证。（PREADY-001、PREADY-006）
  - 2026-10-16：`docs/acceptance/product-loop-baseline.md` 已逐项关联源码、registry 条目、原 change 任务与新增差额；核对正式包 fixture、自导入/手工 token、HA hook 与真实迁移、PG round-trip 与整站恢复的边界，不变更认证。
- [x] 1.2 确认 Portal 仓库、维护者、构建版本与跨仓测试入口，固定后端/前端/CLI 的公开 API 边界；缺仓库时记录阻塞及可独立推进项。（UXLOOP-001、DEVEX-001）
  - 2026-10-16 partial：已确认本仓库 `web/` workspace、Shell 0.1.0、共享 API client、Vitest/Playwright 与 BFF/领域边界，详见基线清单；不存在“缺前端仓库”阻塞。人员维护者、实际构建物料和真实联调入口尚未固定，不将源码或测试入口存在视为验收通过。
  - 2026-10-17 complete：`docs/deployment/portal-api-boundary.md` 固定仓库/维护者/版本/构建/测试/API 边界；`pnpm -C web typecheck`、**65 files/342 tests**、`build`、`boundary:check` 通过；移除 dashboard 直接 `fetch`，新增静态守卫。Playwright E2E、Portal 镜像/部署和 CODEOWNERS 未做，保留为 UI/发布后续，不据此提升 S2/S3 或生产认证。见 `docs/acceptance/reports/20261017-s0-portal-api-boundary.md`。
- [ ] 1.3 固定管理基座、独立业务目标及一个真实 CSI/SC、Gateway controller 的版本/物料/网络条件，登记首轮兼容矩阵和 ≥3 故障域演练环境需求。（RTLOOP-001、DEVEX-002、OBSLOOP-002）
  - 2026-10-16 blocked：基线文档已固定历史报告范围；未获独立业务集群、实际 CSI/Gateway/TLS 与三故障域环境，不从历史管理基座或 Provider 存在推导可用。
  - 2026-10-17 partial/blocked：只读观察到管理基座 K3s v1.36.5+k3s1、真实 TopoLVM CSI 0.42.0（digest `sha256:719a…ce4b`）与 SC `hnb-topolvm`（Retain/WFFC/expand）；Gateway API/controller、外部入口/TLS、Snapshot、独立业务集群和 ≥3 故障域均缺失。兼容矩阵/环境需求见 `docs/deployment/independent-target-baseline.md`，报告 `docs/acceptance/reports/20261017-s0-target-baseline.md`。不完成本任务，S2/S3 实机保持 blocked。
- [ ] 1.4 批准版本化预算：TTL/续期/撤销传播、应用验证超时、冷安装/就绪、API/Operation/投影延迟、各组合资源/每目标增量、稳定性窗口与 RTO/RPO；每项注明数值、单位、负载、窗口和通过规则，未批准项阻断对应认证。（IDENT-001、ADL-002、PREADY-004、OBSLOOP-002）
  - 2026-10-16 pending decision：数值预算尚未批准。1.8 为预算提供严格字段、单位/窗口/样本校验，测试数值不是产品承诺，不填入虚构的 approved 计划。
  - 2026-10-17 proposed：`docs/deployment/decision-baseline.md` 修订 0.1 整理 A1–A17 默认值、量测条件、故障边界与分组批准方式；S1 身份预算可先批，容量绝对值/生产恢复仍需绑定实际环境。本文尚非批准记录，不生成假摘要/approved ReadinessPlan，本任务保持未完成。
  - 2026-10-17 S1 development adopted：用户指示“按规划继续执行”，据此采纳 S1-IDENTITY 为本批开发/测试输入，记录及原基线摘要见 `docs/deployment/service-credential-renewal.md`；生产组和环境绑定仍 pending，不据此完成全项或提升认证。
  - 2026-10-17 status：`docs/deployment/budget-approval-status.md` 明确 S1 仅作开发/测试输入，APP-OPERATIONS、PRODUCTION-BUDGETS、RECOVERY、CAPACITY 仍未批准；缺批准方/有效期/环境条件，不生成 approved ReadinessPlan。
- [ ] 1.5 冻结服务身份/入网、typed 应用组合/动作、旅程 Read Model、统一能力目录/组合/实际安装态及原因字典的增量 Schema/API/事件；分开 releaseVersion/profile/selectedCapabilities/backing，列出兼容规则、owning-plane 数据迁移清单，运行契约/漂移检查。（IDENT-001、IDENT-002、ADL-001、UXLOOP-001、PREADY-002、PUPG-006）
  - 2026-10-16 partial：本批仅新增离线 readiness 计划/预算/证据 Schema，复用现有 identity、console、RuntimeTarget 契约和 BFF delegation；在线身份发行/入网及 typed 应用动作仍需独立冻结，不以离线 Schema 替代本任务。
  - 2026-10-17 proposed：上述决策基线 B1–B11 与 §6.2 列明建议采纳项和实际冻结包；按现有 apiserver/IAM owner 复用签发，conditions 增量兼容摘要，typed union 仅后续 PoC。批准方向不等于 Schema/API/事件/迁移已冻结，本任务保持未完成。
  - 2026-10-17 partial：新增 `service-credential-scope.schema.json`，固定精确 subject/audience/permission、消费者 issuer 接口、验签/范围/有效期与失效恢复语义；契约单测 26/26、全契约兼容/漂移门通过。在线发行/撤销/入网 API、持久化与事件仍在后续 S1 任务，不以消费者契约替代全项冻结。
  - 2026-10-17 partial：S1 身份/入网增量（7 个 identity Schema）与 Console BFF cluster/Operation 契约已冻结，摘要见 `docs/deployment/contract-freeze-status.md`；typed 应用组合/动作、旅程 Read Model、统一能力目录/组合/安装态/原因字典仍未冻结。1.5 保持未完成，不以 S1 子集宣称全量冻结。
- [ ] 1.6 固定参考应用的 v1/v2/故障版本真实 digest、卷读写探针与外部探针并关联可执行发布物料；证据基础设施独立拆到 1.8/1.9，不以 fixture 摘要替代实际应用。（ADL-002、ADL-003、PREADY-001、PREADY-006）
  - 2026-10-16 blocked：未固定实际参考应用物料及外部数据路径；1.8/1.9 的 synthetic fixture 仅验证校验器，不登记为平台或应用 E2E。
  - 2026-10-17 blocked：`docs/deployment/reference-application-baseline.md` 固定所需 v1/v2/故障 digest、卷读写/外部探针与发布物料；仓库仍无真实参考应用 digest，未执行应用 E2E，不把 fixture 当实际应用。
- [x] 1.7 固定统一底座与四档推荐预设矩阵、产品名/既有标识映射和跨档选装/升级断言；核实任何可选能力都不以档位名作功能授权，T0/T1/T2/T3 不映射为 edition，差异只表达默认配置、规模与保障。（GOV-002、PREADY-007、PSHAPE-002）
  - 2026-10-16：`docs/deployment/profile-composition-contract.md` 固定四档/既有标识与 overlay 映射，统一目录、三类演进和八项跨入口断言；核对现有向量共享基础能力而不自动加载 Pack。实际安装态求解与全档位加载仍归 5.3–5.6/6.10/7.9，不以本项替代实现认证。
- [x] 1.8 建立版本化阶段计划、数值预算与证据 manifest Schema，以及只读本地校验入口；绑定预期发布身份/范围、批准预算、报告内容摘要与实际测量，不写 registry 或授予安装资格。（PREADY-001、PREADY-004、PREADY-006）
  - 2026-10-16：新增 `contracts/schema/readiness/v1/product-readiness.schema.json`、`scripts/validate-product-readiness.mjs` 和文档；复用已有 Ajv，`npm run readiness:check` 接通，`executionAuthorized` 永为 false。Schema 在全契约扫描/本地单测中通过；全仓 compatibility/drift 的既有阻塞另行保留。证据见 `docs/acceptance/reports/20261016-product-loop-s0-development.md`。
- [x] 1.9 为 1.8 增加缺报告、摘要不符、旧证据、错误拓扑/自导入、低层证据、预算缺失/超限和失败状态的拒绝测试，验证 CLI 非零退出及无敏感输入回显。（PREADY-001、PREADY-004、PREADY-006）
  - 2026-10-16：`npm run readiness:test` 44/44 通过（含 CLI 通过/拒绝、只读、source/config/范围身份和有界测量），已纳入 `contracts:check`；现有契约单测 25/25 与前端边界测试 18/18 通过。报告同 1.8，fixture 仅认证校验器行为。

退出条件：实施范围、数值预算、owner、环境与版本化契约可追溯；环境缺口按阶段保留 blocked。

## 2. S1 / P0 — 服务身份与 Agent 安全入网

进入条件：S0 的身份契约及预算批准；复用现有身份根、SecretReference、签名和审计能力。

- [x] 2.1 补齐身份/入网所需 owning-plane 增量迁移与原子消费/撤销存储，验证重复迁移、并发消费、版本冲突与跨租户拒绝；无新增字段时附复用证明，迁移标 N/A。（IDENT-001、IDENT-002）
  - 2026-10-17：新增迁移 `105_service_identity_lifecycle.sql`（及 rollback）与 `service_identities`/`service_credentials`/`service_enrollment_credentials` 三表；`ConsumeEnrollmentCredential` 以单条 `UPDATE ... WHERE consumed_at IS NULL AND expires_at > now RETURNING` 保证至多一次消费，身份撤销使用 version CAS。集成测试在 PostgreSQL 16.15 覆盖连续两次迁移、100 并发消费恰好一次成功、版本冲突、重复撤销及跨租户/跨目标/过期拒绝。见 `docs/acceptance/reports/20261017-service-identity-lifecycle.md`。
- [x] 2.2 接通服务身份 issue/renew/revoke 的受限公开契约、独立权限和 actor/service subject 审计，验证 audience/action/tenant/target 越权拒绝与无凭据泄漏。（IDENT-001）
  - 2026-10-17：apiserver 新增 `serviceIdentity` 资源路由（register/list/get/issue/renew/revoke），复用既有 action 枚举；凭据签发/续期/撤销均由 `ServiceCredentialAuthority` 基于存储身份重新计算 scope，撤销权威状态在 DB 而非 JWT 过期。新增 `service-identity-create`/`service-credential` Schema 与契约测试；`audit_logs.service_subject_id` 区分 actor 与服务主体；凭据响应用 `no-store`，审计正文抑制，错误不回显 token。续期暂为持有式 bearer 过渡路径，Projected SA/workload proof 归 2.4/1.5。
- [x] 2.3 实现通用客户端提前续期、有界退避/抖动及在线凭据替换，单测时间边界、过期失败关闭、取消和重启恢复。（IDENT-001）
  - 2026-10-17：新增 `pkg/iam/service_renewal.go` 实现既有 TokenSource：首用启动、后台轮换、固定精确 scope、公钥验签、50%±5% 提前续期、1/2/4s±20% 退避、2s RPC 界限；拒绝过期、越权、永久拒绝/撤销及 issuer 原始错误泄漏。伪时钟验证三个 TTL、100 并发仅一次冷签发、取消/关闭、过期失败关闭/恢复和新 source 重启。`go test -race ./pkg/iam/...` 与 `go vet ./pkg/iam/...` 通过；本项仅交付客户端，真实 issuer/worker 接线、即时撤销和实机演练仍归 2.2/2.4/2.7/2.8。见 `docs/acceptance/reports/20261017-service-credential-renewal.md`。
- [x] 2.4 将 worker→Provider 和 BFF→领域 API 调用切换为正式可续期身份，验证保留 actor、目标 scope 和最小权限，不新增静态 token 回退。（IDENT-001）
  - 2026-10-17：operation-worker 支持 `serviceIdentities` per-tenant 声明 + `SERVICE_IDENTITY_*` 全局配置，经 `NewHTTPTenantTokenSource` 为每个 tenant 建立固定 scope 的续期 source；未声明 tenant 直接拒绝，新路径失败不回落 tokenFile。驱动按 `execution.TenantID` 取凭据。BFF 仍按设计使用每请求 delegation（actor 绑定由既有 `DelegationSigner/Verifier` 测试覆盖），不改成共享 actor 会话。legacy `tokenFile` 仅保留给未迁移 provider；生产 rollout 与真实跨 TTL 演练归 2.8。
- [x] 2.5 接通 Agent 单次 enrollment exchange 与目标绑定，验证并发交换至多一次成功、过期/撤销/跨目标/跨租户及重放拒绝。（IDENT-002）
  - 2026-10-17：新增 `POST /resources/clusters/{id}/agent-enrollments`（15min 单次凭据，仅存哈希）、公共 `POST /api/v1/agent-enrollments:exchange`（原子消费后签发 5min `agent-tunnel/v1` 会话）与撤销端点。`ConsumeEnrollmentCredential` 的 `UPDATE ... WHERE consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now` 保证至多一次；handler 测试覆盖重放、过期、撤销、跨租户/跨目标、错误 target 类型，审计不含明文。Agent 二进制的 exchange/轮换接线归 2.6。
- [x] 2.6 接通 Agent 身份轮换、重连/重新入网与 observer generation fencing，验证旧 identity/generation 不能覆盖新观测。（IDENT-002、RTLOOP-001）
  - 2026-10-17：新增 `agent_sessions`（session/generation/撤销）与 `POST /api/v1/agent-sessions:renew`；exchange 返回可续期 session，`AgentSessionTokenSource` 在 50% TTL 轮换、失败有界、撤销永久停止；Agent 配置支持 `AGENT_ENROLLMENT_SECRET_FILE`/`AGENT_EXCHANGE_URL`/公钥 manifest，重连使用轮换后 token。observer generation fencing 由既有 `Projector.Accept`（旧 generation → `ErrFenced`）及测试覆盖，不因会话轮换重置 generation。
- [x] 2.7 接通撤销对持久连接的重新鉴权/关闭及续期失败诊断，集成验证身份服务故障和批准传播上限，不使无关业务数据面中断。（IDENT-001、UXLOOP-002）
  - 2026-10-17：`DELETE /api/v1/resources/clusters/{id}/agent-session` 撤销该 tenant/target 的全部活动 session 并调用 `TunnelServer.Revoke` 关闭精确匹配的连接；`CloseRevoked` 发送稳定 close reason，Agent 映射为 `ErrSessionRevoked` 并停止重连。续期端点对已撤销/过期/generation 不符统一 401；source 对身份服务临时故障保留旧 token 至 `exp`、过期 fail closed，永久拒绝立即停止。真实 5s/60s 传播测量与身份服务故障演练归 2.8。
- [x] 2.8 执行超过两个初始 TTL、含一次服务/Agent 重启的真实身份演练；记录自动续期、撤销时延、失败恢复与权限隔离报告。（IDENT-001、IDENT-002、PREADY-001）
  - 2026-10-17：本地进程级真实链路演练（真实 HTTP/WebSocket/PostgreSQL 16.15、真实 handler/store/tunnel/session source；4s TTL 跨 >2 TTL，11.93s/3 次重连，generation 保持 1；关闭 source 后用已消费 enrollment 重启并从 0600 session file 恢复，未再 exchange；撤销关闭 3.95ms、新 renew 拒绝总 6.38ms；跨租户 exchange 拒绝）。生产 5min soak、云主机/HA、身份服务真实断网与 60s 多连接传播测量未做，保留为环境依赖。见 `docs/acceptance/reports/20261017-identity-drill.md`。

退出条件：正式身份可持续运行，无需手工 mint token；凭据过期/撤销以及入网重放有真实负向证据。

## 3. S2 / P0 — 独立业务集群纳管闭环

进入条件：S1 通过，独立目标可用，复用现有 import 动作与签名 Conformance 登记。

- [x] 3.1 将正式身份、lifecycle Provider、Agent 与观测源绑定接入既有 import/handoff 流程，补齐受理与真实可用分离的状态契约和查询测试。（RTLOOP-001、UXLOOP-001）
- [ ] 3.2 补齐 import→Agent→能力/节点观测→Read Model 的关联和水位，集成验证重复/乱序/缺口/旧 generation、跨租户/跨目标不更新投影。（RTLOOP-001、IDENT-002）
  - 2026-10-17 complete：既有 import/handoff 链路复用受限 RuntimeIntent+delegation、lifecycle Provider、S1 Agent tunnel/session 与 observer；新增 `ClusterAcceptance`（accepted/available/observationState/acceptedAt），List/Detail 同源，OpenAPI 增量生成。单元 + 真实 PostgreSQL 16 查询测试验证 `REGISTERING+pending` 不显示可用、观测后 `ACTIVE+FRESH` 才 available；契约 28/28、门禁 81 schemas。独立集群实机 import、真实 Agent handoff 与 3.2 乱序/generation 仍待后续；见 `docs/acceptance/reports/20261017-s2-import-acceptance.md`。

- [ ] 3.3 在独立业务集群执行正式 CLI/API import 及幂等重放，验证唯一目标、真实 Agent 认证、能力/节点清单和新鲜度，不直写数据库或使用自导入。（RTLOOP-001、PREADY-001）
- [ ] 3.4 注入 Agent 断连与重连，验证旧状态保留、陈旧提示、持续身份、目标不重复及观测恢复；保留时间线和重放结果。（RTLOOP-001、IDENT-001、IDENT-002）
- [ ] 3.5 在实际 Portal 与 CLI 复验同一纳管旅程、跨租户拒绝和迟到响应隔离，登记 S2 证据；Portal 缺失则 UI 验收保持 blocked。（RTLOOP-001、UXLOOP-001、PREADY-001）

退出条件：独立目标正式纳管并持续观测，执行成功与目标可用两个维度均可验证。

## 4. S3 / P0 — 有状态应用黄金路径

进入条件：S2 通过；新增 action 未经对应 Conformance 不进入可执行矩阵。复用已有组合、Gateway 和存储机制，保留现有 Deployment-only Provider 契约。

- [ ] 4.1 补齐应用交付绑定的 owning-plane 增量迁移/查询，固定 release、target、namespace、revision、资源 UID、PVC/入口和 Operation/观测引用；验证迁移兼容与跨租户隔离。（ADL-001、ADL-002）
- [ ] 4.2 补齐应用组合的规划/依赖排序和能力预检，验证 Namespace/SecretReference 归属、配额、固定快照、未知字段和任意 Manifest 拒绝。（ADL-001、DEVEX-002）
- [ ] 4.3 接通有类型的 Service/工作负载关联动作与最小目标权限，补幂等、CAS/fencing、部分失败及重放测试，不扩大旧 deploy 语义。（ADL-001、ADL-004）
- [ ] 4.4 接通有类型的 PVC 创建/绑定及 SecretReference 挂载引用，补真实卷供给/读写、凭据不回显、越权和失败清理/保留验证。（ADL-001、ADL-004、DEVEX-002）
- [ ] 4.5 接通一个实际 Gateway 实现的受控入口绑定及外部访问探针，验证 controller/CRD 缺失、Route 未生效和网络不可达不误报成功。（ADL-001、ADL-002、DEVEX-002）
- [ ] 4.6 实现应用就绪聚合与有界验证：固定 revision/UID/观测水位，分别展示执行、观测、外部探针及卷读写结果，覆盖成功执行但验证失败的集成测试。（ADL-002、UXLOOP-001）
- [ ] 4.7 接通应用 v1→v2 的版本固定、数据兼容/备份点检查和升级 Operation，验证新版本观测与外部数据读写。（ADL-003）
- [ ] 4.8 接通升级失败后的保留旧版本/关联恢复 Operation，验证不可逆迁移自动降级拒绝，恢复不覆写历史终态。（ADL-003）
- [ ] 4.9 接通删除影响预览、默认卷保留、独立数据清理授权和 generation 墓碑；验证非托管资源拒绝及迟到 deploy 不复活应用。（ADL-004）
- [ ] 4.10 对新增 action 运行真实契约/功能/故障/安全/幂等/fencing/性能 Conformance 后逐项登记矩阵，补充目标侧 RBAC、硬配额及所声明网络隔离的实际允许/拒绝用例。（ADL-001、DEVEX-002）
- [ ] 4.11 在实际 Portal 和 CLI 接通计划预览、应用交付/升级/删除与状态跟踪，验证多入口同义、重复提交、权限和租户切换安全。（UXLOOP-001、ADL-001、ADL-004）
- [ ] 4.12 执行完整黄金路径：租户/配额→独立目标→应用部署→外部持久化读写→升级→故障恢复→删除保留卷，归档实际数据标记和 S3 证据，不提前声明正式包交付。（ADL-001、ADL-002、ADL-003、ADL-004、PREADY-006）

退出条件：真实应用业务路径通过，相关能力及限制逐项可追溯。

## 5. S4 / P0 — 正式发布与基础产品闭环

进入条件：S3 通过；`standardize-hnb-deployment-toolkit` 4.10/5.7/8.1 对应发布物料与文档已具备。本阶段消费这些输出，不重复实现安装器。

- [ ] 5.1 核验真实 CLI/Chart/镜像/迁移/依赖/锁与兼容矩阵一致性，生成产品候选包清单；检查所有 S1–S3 物料摘要对应实际运行身份。（PREADY-001、PREADY-005）
- [ ] 5.2 补齐面向实施者的安装→纳管→应用→升级恢复操作卡，区分能力加载、档位调整与软件版本升级，注明预置条件、缓存、端口、维护窗口、身份/存储绑定、保留/卸载语义及已支持范围。（PREADY-005、ADL-004、PUPG-006）
- [ ] 5.3 从既有配置/锁/run/观测核对实际安装态，保留显式能力、版本、配置覆写及资源/数据引用；补齐必要增量迁移并测试旧配置兼容和未知能力不丢失，不新建第二权威状态库。（PSHAPE-002、PUPG-006）
- [ ] 5.4 将实际安装态接入既有升级 planner，合成“目标必需基线 + 保留能力 + 显式新增”的依赖闭包与配置差异，验证兼容版本求解、资源不隐式下降及可选 Pack 不自动全装。（PSHAPE-002、PUPG-006）
- [ ] 5.5 将源安装态摘要与目标锁接入既有执行/恢复流程，校验并发漂移、身份/引用保留、分阶段可逆性和业务中断预算；backing 迁移复用原变更契约，未认证路径继续拒绝。（PUPG-006、PREADY-005）
- [ ] 5.6 补齐“Minimal + 已选能力→Lite”、同档能力增量、同档 release A→B 的契约/集成测试，覆盖已装组件无兼容解、配置覆写保留、漂移拒绝和恢复不重复；不以默认 overlay 比较代替实际态比较。（PUPG-006、PSHAPE-002、PREADY-007）
- [ ] 5.7 由非该实现开发者在干净已有 Kubernetes 环境仅凭正式包/文档执行首次安装，记录人工步骤、问题和耗时，不沿用历史已运行平台。（PREADY-005）
- [ ] 5.8 在该安装实例执行受控安装中断/续跑及一次保留已选能力、配置和数据的真实平台版本升级/失败恢复，校验 run/锁/源安装态/集群身份；记录维护窗口和持续业务探针，不可逆路径验证明确拒绝。（PREADY-005、PUPG-006）
- [ ] 5.9 由实施者复验 S2/S3 全旅程，核对 UI/CLI 状态和数据标记；失败先修复并重跑受影响断言，再登记基础闭环结果。（PREADY-005、PREADY-006、UXLOOP-001）
- [ ] 5.10 发布 S4 范围报告及支持矩阵：基础闭环与生产/HA/离线分别标注；未完成的 bootstrap/离线仍引用原 change blocked 项，不推导通过。（PREADY-001、PREADY-006）

退出条件：正式发布包在独立实施者安装的环境复现完整基础闭环。

## 6. S5 / P1 — 可运营、按需组合与轻量优化

进入条件：S4 通过。仪表与资源基线可提前准备，但正式验收以固定发布/预算为准。

- [ ] 6.1 从 overlay/manifest 派生基础/容器管理/生产增强组合及依赖闭包，分别输出 releaseVersion/composition/profile/backing/evidenceScope，验证与既有 Minimal/Startup 和外部依赖一致；全部组合可按实际条件在四档组装，不形成第二套封闭功能档位。（PREADY-002、PREADY-007）
- [ ] 6.2 接通各档位组合安装/启用/禁用/升级的增量、容量差异和依赖阻断，复用 extension-controller/Operation；验证无需替换 Core 或重装平台、不强制改档位，活动任务/资源引用受保护且默认数据保留。（PREADY-003、PREADY-007、PUPG-006）
- [ ] 6.3 在 UI/CLI 提供统一能力目录、组合和可用性查询/选择，区分可发现/已装/启用/缺依赖/不兼容；统一原因字典、最后观测时间及可恢复步骤，覆盖后端不可用和租户切换，未安装业务菜单仍遵守能力权限交集。（PREADY-002、PREADY-003、PREADY-007、UXLOOP-001）
- [ ] 6.4 接通受限标准指标/日志配置、查询与健康校验，复用已有 Prometheus 接入；测试共享集群租户标签、任意查询/越权拒绝及后端故障 Unknown。（OBSLOOP-001）
- [ ] 6.5 接通应用/告警→资源→Operation/步骤→日志/指标→Runbook 导航与受控终端，测试逐请求/会话权限、超时/背压和无高级后端的最小诊断。（UXLOOP-002、OBSLOOP-001）
- [ ] 6.6 扩展既有脱敏 collect，加入身份续期、积压、依赖和观测水位；使用凭据/私钥/客户数据负向 fixtures 验证无泄漏，演练从告警定位并修复一次真实故障。（UXLOOP-002、IDENT-001）
- [ ] 6.7 运行固定条件的冷安装、稳定空闲和读写/Operation 混合负载，采集实际内存/CPU、镜像/解包磁盘、延迟、PG 连接和消息积压，按组件范围出报告。（PREADY-004）
- [ ] 6.8 在一个与多个独立目标、断连/重连和批准稳定性窗口下测量 Agent/连接/采集器增量及控制面负载，记录错误/超时/429 和采样限制，形成容量上限。（PREADY-004、OBSLOOP-001）
- [ ] 6.9 根据测量提交优化 ADR 并实施一个证据支持的瓶颈修复，复验预算/隔离/故障恢复；只有证据支持时评估同平面共进程，否则保留独立部署，发布 S5 基线。（PREADY-004、PREADY-006）
- [ ] 6.10 运行四档共用目录/镜像摘要/协议的一致性测试，在资源满足的 Minimal/Lite 实测加载一个已实现的可选能力，并验证缺依赖时返回实际条件而非 edition 限制、加载后档位/认证不被自动改变。（PREADY-002、PREADY-007、GOV-002）

退出条件：用户能完成日常排障，组合边界清晰，轻量收益和规模成本有实测依据。

## 7. S6 / P1 — 真实扩展、HA 与完整恢复

进入条件：S5 通过；HA 和恢复演练有足够独立环境。两个依赖 change 的原任务在原处执行/验收，本阶段做物料与业务范围对接。

- [ ] 7.1 发布最小 SDK/模板、版本化公共契约及本地开发脚本，包含权限、能力、UI Schema/注册、真实 backend action、健康和诊断样例。（DEVEX-001）
- [ ] 7.2 构建参考扩展并验证 install/enable/action/disable，确认不改 Core/Shell、跨租户拒绝与导航/权限交集。（DEVEX-001）
- [ ] 7.3 对参考扩展演练失败升级/回退和依赖阻断卸载，核对物料摘要、旧能力/导航快照恢复及持久数据保留，归档真实生命周期证据。（DEVEX-001、PREADY-003）
- [ ] 7.4 核对所选 Kubernetes/存储/网络入口的逐动作兼容矩阵，接通实际节点故障域、CRD/controller 健康和认证有效期探测，验证仅 CLI 声明就绪不能放行。（DEVEX-002、PREADY-001）
- [ ] 7.5 消费 `in-cluster-ha-backings` 5.4/5.5 的真实 failover/容量报告，在匹配摘要/拓扑环境以应用持续读写探针复验控制面、PG 主实例和 JetStream 节点故障，记录业务影响及 etcd IO 争用；原报告缺失则 blocked。（PREADY-004、PREADY-006、OBSLOOP-002）
- [ ] 7.6 定义并生成完整恢复集合 manifest，关联各 owning plane、PG/消息水位、身份/撤销版本、权威制品及业务卷范围；准备有序已确认写入标记及停写/重放对账步骤。（OBSLOOP-002）
- [ ] 7.7 在隔离的新环境执行整组恢复，验证引用、制品摘要、消息幂等/fencing、撤销不回退、真实登录/外部应用读写；演练缺制品/错恢复点的拒绝。（OBSLOOP-002）
- [ ] 7.8 计算经批准定义的实际 RTO/RPO，分别报告平台与业务数据恢复；按物料/OS/拓扑/profile/backing 更新生产支持范围，原 change 的离线矩阵未通过时维持未支持。（OBSLOOP-002、PREADY-001、PREADY-006）
- [ ] 7.9 在路径已认证的真实环境按批次演练“Minimal + 可选能力→Lite→Standard→Pro”，保留用户能力/配置/身份/数据，测试阶段中断续跑和业务影响预算；未满足某段容量或认证则保留该段 blocked，不用已通过前段推导。（PSHAPE-002、PUPG-006、PREADY-007）

退出条件：只有对应组合的运维、预算、真实扩展、完整恢复和适用 HA 证据均通过才开放生产声明。

## 8. S7 / P2 — 可选 GitOps 与后续生态

进入条件：S4 已通过后可独立启动，不阻塞基础闭环发布。若业务选择延期，本节实现任务保持未完成并登记后继 change；本 change 归档前须通过显式范围变更转移相应 delta/任务，不能把延期勾为已实现。

- [ ] 8.1 选择首个 GitOps 控制器/版本，冻结 owner/revision、受控声明源、观测及权限契约，记录与 HNB Operation 的职责和移交策略。（ADL-005、DEVEX-002）
- [ ] 8.2 实现外部 owner 下只观测或经 Operation 更新受控声明源并等待收敛的适配，不引入第二套运行事实或直接双写目标。（ADL-005）
- [ ] 8.3 集成验证 revision 冲突、移交中断、旧 writer 恢复、越权、撤销与所有权恢复，运行真实控制器 Conformance 后单独开放该集成。（ADL-005、DEVEX-002）
- [ ] 8.4 基于实际客户需求为 DevOps/更多基础设施/边缘建立独立后继 change 索引，写明分级、owner、预算和 Conformance 前置；未实施项不加入支持矩阵。（PREADY-001、PREADY-006、DEVEX-002）

## 9. 验证、阶段证据与最终归档

- [ ] 9.1 检查所有新增 Requirement 到契约、迁移、实现、单元/集成/真实 E2E/回滚报告的映射；跨仓前端、环境或依赖未完成项保持 blocked，按阶段保留历史失败。（PREADY-001、PREADY-006）
- [ ] 9.2 运行受影响模块的格式/build/vet/race、契约/生成物漂移、Chart 组合与 UI 测试，以及 `openspec validate --all --strict` 和 `node scripts/validate-openspec.mjs`；如工作流提供 verify，完成后再申请归档。（PREADY-001、DEVEX-002）
  - 2026-10-16 partial：全仓契约门禁与生成漂移已恢复通过（`validate-contracts` compatibility=checked、`generate-contracts --check` 干净），根因是工作树未提交的 RuntimeIntent node-add 契约，已按纯增量重写；详见 `docs/acceptance/reports/20261016-contract-gate-runtime-intent.md`。归档前的 build/vet/race、Chart 与 UI E2E 仍以固定发布/环境为准，未在此勾选。
- [ ] 9.3 更新用户/实施/运维/扩展开发文档与发布支持矩阵，复核阶段范围及依赖 change 证据，再 archive 合并本 delta；不得在尚有必需 blocked/未实施任务时宣告全部完成。（PREADY-005、PREADY-006、DEVEX-001）

覆盖说明：Schema/API/事件适用，见 1.5 及各实现任务；数据库增量适用，见 2.1/4.1，确可复用时须明确 N/A 原因；无新增必装中间件，其独立安装/升级/备份/卸载实现为 N/A，外部可观测与配套 controller 的生命周期/成本仍须测试。单元、集成、契约、真实 E2E、文档和回滚均适用；已有安装/HA/离线实现由原 change 负责，本变更只验收跨能力产品差额。
