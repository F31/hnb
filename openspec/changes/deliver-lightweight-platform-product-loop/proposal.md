## Why

HNB Minimal 的 19 个工作负载已经在限定的已有 K3s 环境验证启动、登录、读 API、Agent 隧道和滚动恢复，但这不等于独立业务集群纳管、有状态应用交付、持续运维和生产恢复的完整产品闭环。下一阶段应以可重复交付和真实用户旅程为中心，将“轻量敏捷、灵活扩展、简单高效”变成可测量、可验收的产品行为。

四档产品定位共享统一技术底座：Minimal 用于演示、Lite 用于最小生产、Standard 用于常用生产、Pro 用于高级特性与企业生产；这些定位定义推荐默认组合、规模和保障目标，不定义功能壁垒。全部已发布组件与服务能力均进入统一目录，在任一档位可按需加载、组装并连续演进，实际可执行范围由权限、依赖、资源、兼容性与证据决定。

## What Changes

- Change ID：`deliver-lightweight-platform-product-loop`。按 S0 基线、S1 身份、S2 纳管、S3 应用、S4 交付、S5 运维与轻量、S6 生产与生态、S7 可选集成的顺序实施；以阶段门而非 Pod 数判断完成。
- 新增跨能力的产品就绪与验收契约：证据绑定真实发布身份、独立业务目标和批准预算，明确基础闭环与生产就绪的不同退出条件。
- 补齐服务身份签发、续期、撤销、Agent 一次性入网及重连恢复，消除手工短时 token 依赖。
- 打通正式 Intent/Operation 到目标观测的纳管闭环，并在一个带 PVC、SecretReference 和外部入口的参考应用上验证部署、访问、升级、失败恢复和数据保留。
- 定义有类型的应用组合与受控生命周期，不扩张现有 Deployment Provider 的隐含权限，不接受任意 Manifest 作为捷径。
- 将基础底座/容器管理/生产增强定义为可修改的能力组合模板，独立于 Minimal/Lite/Standard/Pro 推荐部署档位；四档使用同一 Core、公共契约、数据模型、扩展机制和发布列车，不按档位派生功能受限的内核或组件。
- 统一目录面向四档开放全部已发布能力；安装/启用仍校验权限、依赖、兼容、资源和认证，缺条件时给出实际缺口，不要求仅为解锁某项功能购买式地或名义上升档。测量空闲/峰值、镜像体积、启动时间、每目标增量和数据库/消息开销。
- 分离能力增减、部署档位演进与软件发布版本升级；基于当前实际安装状态合成目标，在原实例保留可兼容的能力、配置、身份、资源和数据，通过可恢复的增量计划平滑演进，不用目标默认 overlay 覆盖实际安装。
- 统一 UI/CLI 状态、故障关联和脱敏诊断，标准化外部指标/日志接入；完善扩展 SDK、真实参考实现和全生命周期认证。
- 完成正式版本包的独立安装/升级恢复与非开发人员验收，复用已有 HA/离线工作并补全整站恢复证据。
- 将 GitOps/DevOps、更多基础设施和边缘集成作为 S7 单独增量；首个 GitOps 集成必须明确资源写入所有权。

## Capabilities

### New Capabilities

- `platform-product-readiness`：分阶段产品就绪门、能力组合、容量与轻量基线、可重复交付和独立用户验收。
- `application-delivery-loop`：有类型应用组合、实际可用性判定、版本升级恢复、持久数据保留及可选外部控制器所有权。

### Modified Capabilities

- `identity-tenancy`：新增服务凭据自动生命周期和 Agent 一次性入网行为。
- `runtime-target`：新增正式纳管与持续观测的端到端验收条件。
- `portal-experience`：新增 UI/CLI 一致的用户旅程状态与故障导航。
- `observability-dr`：新增标准可观测接入与完整恢复集合的验收条件。
- `provider-conformance`：新增开发者参考扩展和真实集成矩阵的交付要求。
- `deployment-governance`：将四档 BOM 明确为统一底座的推荐部署预设，T0/T1/T2/T3 是交付/成熟度分级而非档位功能授权。
- `profile-shape`：将默认档位偏序与实际能力组合分离，升档保留已选能力及资源覆写。
- `profile-upgrade`：新增跨能力、跨档位和跨发布版本的原实例增量升级与保留契约。

## Impact

- **分级与平面**：身份、执行事实及隔离为 T0；Kubernetes 纳管、应用基础交付、Portal/CLI 和最低诊断为 T1；高级可观测、GitOps/DevOps 为 T2 按需；新增边缘/异构基础设施按 T3 Conformance 后启用。T0/T1/T2/T3 不映射为四档功能许可，Minimal 也可按条件加载 T2/T3，Pro 不强制全装。跨运行治理、市场和制品平面协作；AI 不作为本次闭环前置。
- **依赖 change**：`standardize-hnb-deployment-toolkit` 拥有安装器、发布闭包、离线和既有生命周期实现；`in-cluster-ha-backings` 拥有 PG/JetStream HA 拓扑、再背靠及 HA 故障/容量演练。本变更引用其交付和证据，不复制任务、不提前提升认证。主规格中已有认证行为直接复用。
- **代码与契约**：影响 `cmd/{apiserver,platform-api,operation-worker,cluster-agent,tunnel-server,extension-controller,hnbctl}`、相关 Provider、`pkg/iam`、版本化 contracts、Chart、验收脚本及本仓库 `web/` Portal workspace。前端人员 owner、实际物料与联调环境未确认时对应验收任务保持 blocked。
- **迁移与兼容**：优先复用身份、SecretReference、Operation、观测及发布目录；所需身份版本/入网记录和应用绑定采用 owning-plane 增量迁移。有破坏性契约升级时另行提案；本提案不删除旧 API、不重命名现有档位、不赋予旧 Provider 新能力。产品名称 Minimal/Lite/Standard/Pro 与既有 Minimal/LiteHA/StandardHA/Enterprise 标识兼容映射，已安装能力不因升档被目标默认清单抹除。
- **回滚**：禁用新增能力、恢复兼容版本和注册快照；保留审计、凭据撤销记录、资源所有权及 PVC。不可逆 schema/数据写入走前向修复或验证后的整组恢复，不承诺任意数据库降级。
- **资源与依赖**：复用 PG、JetStream、OCI 及现有外部服务，不新增必装中间件；采集器/指标/日志后端按需接入。S0 先批准测量条件和预算，不将 requests 当实测，不以 19 负载为总环境成本。
- **安全与可观测**：按 tenant/target/audience 最小权限、短期身份与受控撤销、SecretReference、单写入所有权及关联审计；暴露续期失败、投影延迟、Operation 滞留、依赖故障和恢复水位，不输出 token/私钥/业务载荷。
- **非目标**：不全量复制竞品功能、不自研指标/日志/CI 引擎、不强制合并进程、不将 Startup 冒充完整 Minimal、不把平台备份等同业务数据备份、不把方法级/fixture 验证等同生产认证。
- **退出判据**：S4 完成正式包安装→独立目标纳管→有状态应用交付→一次版本升级与故障恢复的基础闭环；S6 另需日常运维、资源预算、真实扩展、完整恢复和对应 HA 档证据。S7 可选集成单独声明支持范围；所有阶段的通过与阻塞均留存报告。
