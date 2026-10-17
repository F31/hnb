# HNB 产品闭环决策基线

| 元数据 | 值 |
|---|---|
| 基线 ID / 修订 | `hnb-product-loop-decisions` / `0.1` |
| 状态 | **S1 开发输入已采纳；其他组仍 Proposed，非生产验收结果** |
| 编制日期 | 2026-10-17 |
| 关联变更 | `deliver-lightweight-platform-product-loop` |
| 关联任务 | 1.4 数值预算、1.5 契约冻结；后续 S1–S7 按批准范围消费 |
| 约束来源 | 已有主规格、该变更 delta、现有身份/表所有权及公共契约治理 |

## 1. 用途与批准边界

本文把先前讨论收敛为可批准、可实施的建议，区分四类信息：

- **现有约束**：当前代码或已生效契约的边界，不能用本文建议直接覆盖。
- **建议采纳**：本轮推荐的架构方向和配置默认值，尚未表示已经实现。
- **待实测目标**：拟用来验收的目标，不是平台已经达到的性能或 SLA。
- **后续决策**：需要具体目标环境、物料或兼容性研究才能冻结的事项。

批准可按组进行。批准 S1 的身份参数和架构方向后，可以冻结对应 Schema/API 并推进本地实现，
不必等待高级功能或整站灾备环境；真实身份演练仍须有相应环境。1.4 的全量完成需要所有适用
预算绑定环境并批准，1.5 的全量完成需要源 Schema、API/事件、迁移和兼容检查实际完成。
**编写或批准本文不会自动勾选两项任务，也不会提升安装、HA 或生产资格。**

四档坚持同一技术底座、同一功能目录：Minimal 演示、Lite 最小生产、Standard 常用生产、
Pro 高级/企业生产。本文阈值按实际组合、拓扑和负载适用，不构成档位功能授权；Minimal 选装
高级能力不自动成为生产档，Pro 也不强制全装组件。详见
[四档组装契约](profile-composition-contract.md)。

## 2. 对前述“最佳实践”的复核与修正

| 原建议/容易产生的理解 | 本基线采用的修正 |
|---|---|
| 服务身份统一采用 5–15 分钟 TTL | HNB access/service token 和 delegation 当前上限均为 **60s**。先保留；Observer、Agent 与 enrollment 使用各自 profile，不能混用通用时长。 |
| BFF 只转发，发行一律迁到 platform-api | 逻辑权威应集中于 IAM，但当前 IAM 签发在 `cmd/apiserver/main.go`，身份/密钥表 owner 也以 apiserver 为写入方。S1 复用该 owner，不为形式上的分层迁移签名私钥或另造发行者。 |
| 短 TTL 就等于立即撤销 | 离线 JWT 验签不能感知即时撤销；短 TTL 只限制剩余有效时间。5s 撤销目标需权威版本/撤销检查和有界缓存，长连接另需失效机制。 |
| 60s 令牌可统一容忍 ±30s 时钟偏差 | 这会显著扩大有效窗口。现有验证器严格校验 `iat/nbf/exp`；维持现有语义，不自动加入过期宽限。时钟同步另有运维目标。 |
| Token Exchange 自带一次性消费和级联撤销 | RFC 8693 不自动保证二者。HNB 必须明确单次消费、重放防护、子身份撤销及连接失效语义。自定义 JSON 入网接口不能直接宣称 RFC 8693 兼容。 |
| Kubernetes bootstrap token 默认就是一次性 | 不能如此推导。HNB 的单次入网必须由原子消费记录保证；Projected ServiceAccount Token 也有自己的受众、期限和验证方式。 |
| `oneOf + discriminator` 天然无破坏、必然需要 v2 | 两者均不成立。`discriminator` 是辅助分派，校验仍靠各分支；新增分支的响应兼容和生成器行为仍需验证。是否升主版本按真实 wire/语义变化判断。 |
| Schema 不参与运行时校验，所以精确契约没有价值 | Schema 仍影响 SDK、外部校验器和消费者。当前方案 B 只是经验证的工具兼容取舍，不是放弃 Schema First，也不是以后所有 kind 都只在引擎校验。 |
| conditions 取代所有状态机 | conditions 适合目标/应用的观测摘要；Operation、审批、凭据消费仍有明确状态机。二者互补，不得改写历史终态。 |
| 三实例 CNPG、JetStream R3 自动保证 RPO=0 | CNPG 同步复制默认未开启；NATS 三个 server 不代表每个 stream/consumer 都是 R3。同步提交、PubAck、存储持久化、故障模型和恢复点都必须分别核验。 |
| 所有关键 Pod 都应 Guaranteed，requests 等于 P95 | 并无统一最佳设置。按实测工作集、突发、CPU 节流和 N-1 容量分别设定；PDB 也不防止节点硬故障或自动保证 HA。 |
| 各种 TTL、72h soak、15min RTO 都是行业标准值 | 这些是 **HNB 建议目标**，不是 OAuth、Kubernetes 或存储产品的强制默认。已有几十秒只读压测不足以认证生产容量。 |

## 3. A1–A7：身份与入网预算（建议批准组 `S1-IDENTITY`）

### 3.1 令牌类别与授权边界

| 类别 | 当前事实 | 建议默认/上限 | 用法与约束 |
|---|---|---|---|
| BFF 请求 delegation | `MaxDelegationTTL=60s`；当前 BFF 使用 30s | **默认 30s，上限 60s** | 每次下游调用生成，绑定 actor、tenant、audience、action 和语义摘要；不把单次 delegation 当可续期会话。 |
| worker/Provider service access token | `MaxAccessTokenTTL=60s` | **默认/上限 60s** | 通过受信 workload identity 重新取得短期令牌，不仅凭一个旧 access token 无限续命。 |
| Observer 身份 | `MaxObserverTokenTTL=10min` | **默认 5min，上限 10min** | 精确绑定 tenant/target/observer/generation；续期不应无故重置 generation，重新入网才按协议建立新代次。 |
| 新 Agent 会话身份 | 现 `agent-tunnel/v1` 可签发至 24h | **新协议建议默认 5min，上限 10min** | 与旧 profile 分开演进，自动轮换；不是直接修改旧 verifier 的上限。 |
| 单次 enrollment credential | 尚待 S1 契约/存储实现 | **TTL 15min，默认至多一次成功消费** | 高熵凭据只存哈希/受控引用，绑定 tenant、target、预期实例/公钥；不能直接用作长期隧道凭据。 |

同一集群的服务可优先用受众受限、自动轮换的 Projected ServiceAccount Token 证明 workload
身份，经受信 issuer/audience/对象绑定核验后换取 HNB 身份；它不是可以直接传给 HNB verifier
的同一种 JWT。跨集群 Agent 初次信任来自受限 enrollment，之后使用登记密钥证明或受管会话。
SPIFFE/SPIRE、mTLS 和节点证明可作为后续适配，不新增为 Minimal/Lite 的必装依赖。

### 3.2 客户端与撤销的明确默认

| 对应项 / 预算 ID | 建议值 | 判定/实施说明 |
|---|---|---|
| A2 `identity.renewal-start` | 到寿命 **50%** 时启动，启动时刻抖动 ±5% TTL | 60s 令牌在约 27–33s 开始；不用此前“只剩 25%”再启动的较紧窗口。不适用于每请求 delegation。 |
| A2 `identity.renewal-retry` | 退避 1s、2s、4s，上限 4s，抖动 ±20%；单次 RPC 超时 2s | 尊重取消/单次 deadline；singleflight 合并并发续期。下一次尝试必须能在硬到期前完成，否则停止受保护调用，不能自动扩大 TTL/权限。 |
| A3 `identity.revoke-new-call` | 从权威撤销事务提交开始，**≤5s** 拒绝新受保护调用 | 撤销状态正向缓存建议最长 2s；过期且无法核实则拒绝。只发送异步事件或只等 JWT 过期不满足本目标。属于待实现/实测目标。 |
| A3 `identity.revoke-connection` | 同一起点，**≤60s** 使旧身份长连接失效 | 有效期边界强制重新鉴权/关闭，并有周期性撤销检查（建议周期 ≤30s）；定时检查不能代替 `exp` 硬截止。证明失效可停止接收受保护数据，不要求每分钟重建 TCP。 |
| A4 `identity.clock-offset` | 节点与参考时钟绝对偏差目标 **≤1s** | 使用时间同步和偏差告警；当前 token 验证不增加 `exp` 宽限（leeway=0）。若要修改 `iat/nbf` 容差，必须单独评审并计入撤销/有效期上限。 |
| A6 `identity.rotation-overlap` | 同一逻辑身份的新旧凭据重叠 **≤min(30s, 旧凭据剩余有效期)** | 不延长旧 `exp`；明确撤销优先于重叠。签名公钥可为验证未过期 token 保留更久，不能与私钥签发期/凭据重叠混为一谈。旧 observer generation 不因重叠重新获准写入。 |
| A7 `agent.heartbeat-period` | **30s** | 心跳只证明连接存活，不证明资源清单已新鲜；重连指数退避 1s 至 30s 并抖动，不能重复创建 RuntimeTarget。 |

不为工作负载规定“24h 到点必须人工重新入网”。自动重新鉴权、受信身份轮换和一次性 bootstrap
是不同生命周期；采用 HNB 稳定 ID，不把 Pod 重建等同于集群身份丢失。撤销传播、过期处理不能
导致绕过身份检查，但也不能删除已运行客户业务或把管理面失联误判为业务已停止。

### 3.3 S1 最低测试条件

- 集成演练至少持续 `max(30min, 3×最长适用身份 TTL)`，包含服务/Agent 重启；伪时钟单测
  覆盖过期前后边界，不能代替真实续期与连接失效演练。
- 新调用/连接撤销各至少 **100 次**（含空闲长连接、活动连接、多个 verifier、缓存重启与
  撤销存储不可达），每次均不超过 5s/60s，不只报 P95；以单调计时器记录事件差值。
- 同一 enrollment 至少 **100 个并发交换请求**，最多一次产生新身份；同一合法客户端的
  幂等重试不得再次创建身份。响应丢失的恢复、消费记录和授权绑定须在协议中明确，不能简单
  用“同 token 可无限重试”代替单次消费。
- 负向测试含跨租户/目标/audience、令牌 profile 混用、过期、重放、撤销后的续期、旧 generation
  回放及日志脱敏；每次测试保留命令/响应类别与时间，不保存 token/私钥原文。

## 4. A8–A16：应用、运行与容量目标

下表是**候选验收目标**。硬件、目标数量、制品大小和测试负载未绑定的行不构成已批准预算。
各档位可选择同一能力；规模与保障目标只按实际组合设置。

| 对应项 / 预算 ID | 建议目标 | 负载/计时/采样与通过规则 |
|---|---|---|
| A8 `app.verify-deadline` | 总验证超时 **600s** | 从当前 revision 的相关资源变更 Operation 完成开始，到新鲜观测、持久化读写与外部探针全通过；资源 300s/观测 120s/外部 180s 是诊断子预算，可并行，总时限仍为 600s。至少 10 次，均在时限内或返回明确失败。 |
| A9 `observation.freshness` | 资源观测 age **≤60s** 为 fresh；>60s 为 stale；无认证心跳 ≥90s 标记连接不可达；陈旧 ≥300s 告警 | 每 10s 检查，连续 30min 并注入断连。连接与资源观测时间分别记录；过期观测不能因收到心跳变 fresh；时间来源不可信时 Unknown。 |
| A10 `app.external-probe` | 间隔 **10s**，单次超时 **3s**；2 次连续成功确认可用，3 次失败确认降级 | 从业务集群外探测正确版本和内容，不只看 HTTP 200。连续阈值是状态防抖，原始失败仍计入统计，不修改 Kubernetes 自身 readiness。 |
| A11 `app.volume-probe` | **≤30s** 完成小数据写入、适用的 flush/fsync、回读和标记比对 | 只写参考应用授权目录；部署/恢复后复验。使用唯一标记，至少 10 次；不能以挂载成功代替数据恢复，也不在客户卷任意覆盖文件。 |
| A12 `install.ready` | 参考 Minimal 的安装命令启动→发布清单全部就绪，目标 **P95≤1800s** | 至少 20 次独立安装（少于 20 次只报样本，不认证 P95）；分别报告镜像冷/热缓存与迁移时间，逐秒计时、5s 采样。以 `P[k]=sorted[ceil(k×N)]` 的 nearest-rank 统计；不能用暖缓存通过声明断网/冷缓存已通过。 |
| A13 `api.latency` | 沿用 [`performance-budgets.md`](../performance-budgets.md) 的端点级 P95（列表 <200ms、提交 <500ms 等）；参考补充目标 P99≤500ms（读）、≤1000ms（提交） | L-READ：8 并发客户端、合计 16 req/s、pageSize=20、响应≤64KiB、同局域网；预热 5min、稳定测量 30min、逐请求记录。登录单独测，不以高频登录代替业务读写。超时/429/5xx 和分位数分开完整报告；既有更严预算不静默放宽。 |
| A14 `operation.dispatch` / `projection.lag` | 计划已提交且批准、依赖就绪→首个可调度步骤 **P95≤5s**；权威接纳观测→读模型 **P95≤10s** | L-MIX：L-READ 加每分钟 6 个已认证参考 Operation，至少 30min；调度、远程执行和人工审批等待分开计时。队列积压/失败样本必须可见，不把所有步骤完成统一承诺为 30s。 |
| A15 `capacity.budget` | 参考设计保留约 **30%** 稳态 CPU/内存余量，实际 N-1 场景逐档验算；每 Agent 候选目标 P95 CPU≤100m、内存 working set≤128MiB | 中心/底层集群/目标侧/外部服务分列；idle 稳定 30min、L-MIX 至少 30min、5s 采样。CPU 为真实区间增量，内存同时列 working set/RSS；按同一时刻合计。Agent 目标须绑定 1/10 个实际目标和真实资源清单，未达到规模不认证外推。 |
| A15 `postgres.connections` | 各进程应用池最大值合计 **≤70% 可用连接容量** | 可用容量为实际 `max_connections` 扣除系统/维护保留；5s 采样并分 active/idle/系统。不采用“所有档位≤100”常数，也不因堆积超限自动新增 PgBouncer；如实测需要，再作为能力/依赖显式接入。 |
| A16 `stability.soak` | 生产组合 **72h**，非预期重启 **0**，正常负载非预期失败率 **≤0.1%** | L-MIX 与批准的资源规模；指标 5s、健康/探针 10s。24h 只能作为开发预演，不降低 Lite 的生产门。计划注入故障/发布单列窗口及恢复结果；超时/429 不从正常流量分母悄悄移除。固定负载下稳定后首/末 12h 内存中位数增长候选上限 5%，解释缓存/业务增长，不只看最终存活。 |

### 4.1 参考硬件与仍需填实的边界

供批准的**测试起点**（不是已认证最低配置）：Minimal 管理节点 4 vCPU/8GiB/100GiB SSD；
Lite 为三个独立控制故障域、每节点同规格；Standard/Pro 的测试起点为三域每节点
8 vCPU/16GiB/200GiB SSD。业务目标独立部署，初始起点 2 vCPU/4GiB，磁盘、Pod 数和
StorageClass 由参考应用配置确定。真实环境的 CPU/SSD/网络延迟与带宽必须填入计划，
不能只写“同等配置”；overlay、实际启用能力和 external backing 开销均须纳入。

全组合的 idle/峰值内存绝对值、镜像下载/解包字节、最大目标/对象数尚无足够测量，仍为后续
决策；不使用 requests 或此前 Startup 只读压测推导四档容量。资源绝对预算在 1.3 环境和
1.6 参考应用确定后批准到实际 cell，这些缺口不阻断已批准的 S1 契约/本地开发。

## 5. A17：恢复目标，按故障模型而非产品名承诺

| 预算 ID / 场景 | 候选 RPO / RTO | 必要前提与验收 |
|---|---|---|
| `recovery.pg-single-domain`：丢失一个 PG 故障域 | 已确认提交丢失 **0**；业务恢复 **≤900s** | 三域；显式同步提交/远端持久化策略、合适的晋升规则、同步副本不足时阻断写入而非静默转异步；演练核对 WAL/提交标记。CNPG 三实例默认不满足这些条件。 |
| `recovery.js-single-domain`：丢失一个消息故障域 | 指定故障模型下已 PubAck 消息丢失 **0**；服务恢复 **≤900s** | 实际 stream 与适用 durable consumer 的 R3、File 存储、剩余法定人数；检查保留策略/去重及客户端重试。不得推导为多机同时断电、所有盘损坏或整站丢失也为 0；落盘 sync 策略/磁盘语义需专门验收。 |
| `recovery.platform-site`：隔离新环境整组恢复 | 候选 **RPO≤300s、RTO≤7200s** | 初始小规模恢复集合上限建议 **20GiB**（平台元数据、消息、身份材料、所声明制品字节）；使用独立故障域的可验证备份/WAL/消息恢复点。恢复到实际登录、授权、观测和应用外部探针，非只到 Pod Ready；数据量/带宽变化需另定预算。 |
| `recovery.business-data`：客户业务卷/数据库 | **按应用单独批准** | 不继承平台恢复目标。参考应用必须验证其数据标记；客户数据库、外部存储、跨域复制的纳入/排除必须明示。 |

每种故障至少三次独立演练，记录每次及最坏值，不能仅报平均值。RPO 用故障前已确认的
有序业务标记核对；RTO 从故障注入/真实中断开始，到外部探针连续通过（按 A10）结束。
备份型恢复另记故障检测、人工响应、传输、重放耗时；若只计“开始恢复后”的耗时，不得
命名为本表端到端 RTO。复制不替代备份，备份存在不等于恢复可用。

## 6. B1–B11：契约与架构建议采纳项

“建议采纳”表示推荐审批方向，不表示接口、迁移或对应功能已经实现/冻结。

| 决策 | 本基线建议 | 必须保留的边界 / 冻结输出 |
|---|---|---|
| **B1 IAM 权威归属** | 保留 IAM 逻辑 owner，首轮复用 apiserver 当前发行器、密钥与身份存储；BFF 路由和发行逻辑在模块上分离 | 与 [`table-ownership.md`](../table-ownership.md) 和 `cmd/apiserver/main.go` 对齐。platform-api/Provider 验证身份和自己领域权限，不复制 IAM 私钥或直接写身份表。将来拆进程需独立迁移决策，不新增为轻量档必需服务。 |
| **B2 版本兼容** | 能证明兼容的 v1 增量优先；确有破坏性才并行引入新主版本 | 保持路径/operationId/字段语义；分别检查请求、响应、事件及生成 SDK。新增响应枚举也可能影响客户端；`x-extensible-enum` 是工具扩展，不替代消费者审查。门禁绿是必要条件，不是兼容性的充分证明。 |
| **B3 入网与交换** | 新增单次 enrollment、受信续期、撤销与 reenroll profile，参考 Token Exchange 的 actor/audience/scope 思路 | API 仍需明确鉴权方式、原子消费/幂等恢复、PoP 绑定、事件与错误。若要 RFC 8693 互通须完整实现标准 token endpoint/grant/错误语义；本轮不强制新建 OAuth 授权服务器。 |
| **B4 应用绑定** | 优先扩展现有 CompositionRelease、资源绑定与读模型 | 单一 owner 保存 release/target/namespace/UID/revision/PVC/SecretReference/Operation/观测水位；仅在缺失实体时增表，不造第二调度器/事实源。 |
| **B5 typed actions / 入口** | Service、PVC、挂载引用及 Gateway/HTTPRoute 逐动作类型化；首选 Gateway API | 复用现有 gateway-provider、CSI 与实际 controller；不强制移除已有 Ingress，不把 Gateway API 等同外部 LB/DNS/TLS 已就绪。先固定一种实现/版本再开动作，禁止任意 YAML 或客户端提供执行步骤。 |
| **B6 观测条件** | 在目标/应用读模型增量加入 `conditions`，保留需要的兼容摘要 | `type/status(True/False/Unknown)/reason/message/observedGeneration/lastTransitionTime`，按 type 唯一；缺条件按 Unknown，版本陈旧不能 Ready。Ready 从相关条件与引用计算，不是任意 True 的合取。Operation/审批/凭据终态与状态机保留。 |
| **B7 目录与实际态** | 统一目录 + 已选能力 + 依赖闭包，复用 release lock/config/run | 分开 releaseVersion/profile/composition/backing/config digest/evidence；四档全能力按条件可装，不自动全装。升档/同档软件升级保留实际选择和兼容配置、身份与数据，固定源摘要并检查漂移。 |
| **B8 原因与错误** | 延续已有 Problem Details 与稳定 code 字典，参考 RFC 9457 | 保留现有 `UPPER_SNAKE_CASE` API code，不为跟随 K8s 风格重命名；新增 Condition.reason 可用 PascalCase 并维护显式映射，message 可本地化。OAuth 标准端点若引入，保留其标准错误格式。 |
| **B9 迁移** | owning-plane expand-first，再兼容读/回填，最后独立收口 | 列迁移、索引/锁影响、重复执行、撤销版本及恢复策略；**不默认双写两个权威库**。不可逆迁移声明前向修复/整组恢复，不承诺数据库随意降级。 |
| **B10 参数类型与后续联合** | 本轮保留方案 B；公共契约尽量精确，引擎负责最终授权与逐 kind 验证 | 当前标量兜底及 AddNodes 结构例外只是已实现起点。Create/应用配置等合法结构化输入需逐动作 allowlist、界限及兼容测试，不能把“全局只许标量”固化为通用最佳实践。`oneOf + discriminator` 先做消费者/生成器/差分 PoC，再另行决定是否采用、是否需要 v2，不作为 S1 前置。 |
| **B11 产品命名** | Minimal/Lite/Standard/Pro 展示名与 Minimal/LiteHA/StandardHA/Enterprise 机器标识兼容映射 | 复用 [`profile-composition-contract.md`](profile-composition-contract.md)，不重命名历史锁/数据库 ID，不按档位裁剪组件。 |

### 6.1 入网兼容窗口建议

旧 24h `agent-tunnel/v1` 路径采用**一个明确发布窗口，候选最长 30 天**的受控迁移期：
升级先支持新旧 verifier，部署新 exchange/轮换，再迁移 Agent；记录未迁移对象，默认对新目标
使用新路径。窗口结束需管理员核对后关闭旧签发及受理，不能在未获批准的日期自动使现网
Agent 全部失联。保留协议切换的回退步骤，但不得为了回退恢复已撤销身份或新开无期限 token。

### 6.2 1.5 必须提交的契约冻结包

批准架构方向之后，开发仍须提供以下实际产物，不用本文的描述冒充冻结：

| 范围 | 必须固定的内容 | 验证 |
|---|---|---|
| S1 service identity | issue/renew/revoke 的路径、method、请求/响应/错误、身份 profile/claims、audience/权限、PoP/续期证明、大小/速率界限、撤销版本和缓存边界 | Schema + OpenAPI + 生成 SDK；边界、重放、权限及旧客户端测试 |
| S1 Agent | enrollment/exchange/reenroll 的绑定、消费状态机、并发与响应丢失恢复、observer lease/generation、连接重鉴权、旧路径迁移 | owning-plane 迁移、原子性/重放测试、真实长连接撤销与轮换 |
| S3 application | 组合/资源绑定、typed action 名与输入输出、依赖顺序、健康验证和恢复条件 | 逐动作 Conformance 与实际 CSI/Gateway 依赖，不扩大既有 deploy 动作 |
| S3/S5 状态 | conditions/兼容摘要、观测版本/水位、错误映射、能力目录与实际安装态 | UI/CLI 同义与租户切换、陈旧/乱序/缺口场景 |
| 全范围事件/迁移 | schemaVersion/eventId/actor/service subject/tenant/资源关联、outbox、脱敏字段、owner 与迁移清单 | 旧新读写兼容、生成漂移、破坏性检查与受控恢复；身份撤销不只依赖异步事件 |

## 7. 批准记录与阶段推进

### 7.1 推荐批准分组

| 组 | 需要确认的集合 | 批准后可推进 | 仍然不代表 |
|---|---|---|---|
| `S1-IDENTITY` | A1–A7、§3.3，B1–B3、B8–B11 的 S1 部分及旧路径窗口 | 对应 1.5 子集的实际契约包；S1 存储、客户端和服务本地实现/集成 | 1.4/1.5 全部完成、实机认证或 5s 撤销已达到 |
| `APP-OPERATIONS` | A8–A14，B4–B7，Gateway/CSI 选型与实际目标信息 | S2/S3 应用与状态契约、参考计划和真实交付演练 | 仅有 Provider 即可交付应用 |
| `PRODUCTION-BUDGETS` | A15–A17、参考硬件、数据量/带宽及故障模型 | S5 容量/稳定性与 S6 HA/整组恢复验收 | 所有档位/环境同一规模、所有故障 RPO=0 或对外 SLA |
| `TYPED-UNION-RESEARCH` | B10 的后续 PoC 范围 | 新 schema/SDK/兼容性研究和必要的后继 change | 已决定替换 v1、必然需要 v2 或不影响现有客户端 |

建议批准 `S1-IDENTITY` 为下一批开发输入；其余组允许逐项调整后另批。
独立业务集群、CSI/Gateway、三域和备份目的地仍按 1.3/1.6 固定，但不能用其缺失阻止
无关的本地契约开发，也不能省略相应实机验收。

### 7.2 当前记录与批准格式

2026-10-17 用户指示“按规划继续执行”，据此采纳 **S1-IDENTITY 的开发输入**；数值作为
实现与测试目标，不声明已达成。采纳前文档摘要、记录时间、排除项和本批消费者契约见
[S1 服务凭据续期](service-credential-renewal.md)。其他组仍 pending，实际发布/环境与生产
预算尚未批准。后续完整批准应登记基线摘要、组/条目、修改/排除项、批准人、UTC 时间、
适用发布/环境和复审条件。以下仅是未批准组的**记录格式**，不是 ReadinessPlan：

```yaml
baseline_id: hnb-product-loop-decisions
revision: "0.1"
status: proposed
approved_groups: []
approved_by: null
approved_at: null
```

收到明确批准后再填真实身份与时间；修改已批准条目要递增修订并保存旧记录。发布/环境变化、
协议主版本变化、实测超预算或故障模型扩展均触发复审，不默默放宽阈值。

### 7.3 写入机器可核对的预算

- 从本文件生成/填充各阶段的 `ReadinessPlan.budgets[]`：稳定 id、metric、limit、comparison、
  unit、windowSeconds、sampleIntervalSeconds；精确绑定 identity/configurationDigest、
  scope.conditions、故障域和 requiredAssertions，不填假摘要。
- 一次性事件（撤销、入网、安装、恢复）需要事件时间线与原始报告；采样字段不能把一个总时长
  冒充 N 个样本。当前通用校验器不能表达所有事件/分位数语义，缺的部分用明确 assertion
  和可复现报告核对，必要时增量扩契约，不能伪造 sampleCount。
- 保留逐请求分布、错误分类和演练最坏值。`contentVerified=true` 只代表离线内容/预算匹配，
  不代表报告真实性验证、执行授权或跨阶段晋升。
- 对应工具见 [阶段证据契约](../acceptance/product-readiness-manifests.md)。本次不生成
  `approval.status=approved` 的机器计划，不改验收 registry 的支持范围。

## 8. 事实与参考

### 本仓库事实源

- 身份上限及校验：`pkg/iam/{token,service,delegation,observer_token,agent_tunnel_token}.go`。
- 当前发行入口：`cmd/apiserver/main.go`；表 owner：[`table-ownership.md`](../table-ownership.md)。
- 兼容政策：[`公共契约开发与演进指南.md`](../公共契约开发与演进指南.md)，工具锁在
  `contracts/toolchain.lock.json`；RuntimeIntent 与引擎参数校验不视为相互替代。
- 实测边界：[`product-loop-baseline.md`](../acceptance/product-loop-baseline.md)、
  [`minimal-footprint-audit.md`](minimal-footprint-audit.md)、
  [`20261013-startup-capacity-under-load.md`](../acceptance/reports/20261013-startup-capacity-under-load.md)。
- PG Chart 当前只凭三实例并不自动开启同步提交；NATS 三服务器与 stream/consumer replica
  设置是不同层。以部署物料及实际查询结果为准。

### 外部依据（本次复核，不视为 HNB 已认证配置）

- [RFC 8693 Token Exchange，§2.1/§2.2](https://www.rfc-editor.org/rfc/rfc8693.html)：
  token 交换不自动使输入失效，也不自动形成级联撤销。
- [RFC 7519 JWT，§4.1.4/§4.1.5](https://www.rfc-editor.org/rfc/rfc7519.html)：
  `exp/nbf` 与可选的小幅时钟容差；标准没有为 HNB 规定统一 TTL。
- [Kubernetes ServiceAccount token 配置](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/)：
  projected token 的受众、期限与轮换；HNB 仍须验证信任边界。
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)：
  spec/status、conditions、observedGeneration、乐观并发。
- [OpenAPI 3.1.1 discriminator](https://spec.openapis.org/oas/v3.1.1.html#discriminator-object)：
  discriminator 不改变 schema 的验证结果，不能代替分支校验与兼容测试。
- [CloudNativePG 1.24 replication](https://cloudnative-pg.io/documentation/1.24/replication/)：
  默认异步；显式同步及同步副本不可用时的阻写语义。
- [NATS：Surviving node loss](https://docs.nats.io/learn/jetstream/surviving-node-loss.md)：
  stream R3、PubAck 与存储持久性的区别；实际验收仍固定 HNB 所用 NATS 版本。
- [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)、
  [Google SRE：Implementing SLOs](https://sre.google/workbook/implementing-slos/)：
  错误协议及以测量/业务需求制定 SLO，不提供本表数值的通用保证。
