## ADDED Requirements

### Requirement: [PINST-001] 初次管理基座初始化边界
首次初始化 SHALL 仅用于显式指定且可确认尚未由 HNB 纳管的 HNB 管理基座和自身平台组件。工具 SHALL 区分新建管理 Kubernetes 与在已有 Kubernetes 中安装平台；后者 SHALL 保留外部集群生命周期 owner。初始化 SHALL NOT 修改业务 RuntimeTarget，不得因平台故障自动进入该模式；边界或所有权无法确认时 SHALL 拒绝执行。

**Traceability:** KERNEL-002, P1-WRITE-004, RT-009, DPLY-006

#### Scenario: 已有集群安装 HNB 平台
- **GIVEN** 运维授权在一个外部管理的 Kubernetes 中首次安装 HNB
- **WHEN** 选择已有集群路线
- **THEN** 安装仅拥有本次平台资源并保留原集群/CNI 所有权
- **AND** 不执行节点初始化或重置

#### Scenario: 已纳管目标请求本地初始化
- **GIVEN** 目标具有已建立的 HNB 管理关系或其他禁止覆盖的管理基座身份
- **WHEN** 工具收到本地 bootstrap 请求
- **THEN** 工具在副作用前拒绝并指明正确管理入口
- **AND** 不复用该模式规避 Operation

### Requirement: [PINST-002] 完整档位依赖安装
平台安装 SHALL 根据获认证档位交付全部必需组件、Portal、平台/市场执行能力、生命周期 Provider、消息/数据库、入口和持久化配置；可选能力 SHALL 按其分级独立启停。已有基础设施 SHALL 通过已授权连接引用接入，安装器 SHALL NOT 覆盖客户已有基础设施或将组件未部署伪装成能力可用。

**Traceability:** KERNEL-001, GOV-001, GOV-002, GOV-008, RT-010

#### Scenario: 安装 Minimal 档位
- **GIVEN** 物料锁认证 Minimal 的必需组件与依赖
- **WHEN** 执行平台安装
- **THEN** 全部必需组件按依赖顺序安装且可选重型组件保持关闭
- **AND** 未部署的 Provider 在能力列表中标记不可用

### Requirement: [PINST-003] 初始身份和各平面凭据配置
安装 SHALL 为实例初始化有效的身份、服务认证、Secret 加密、消息连接和管理员入口，或接入已授权的外部凭据引用。生产 SHALL NOT 使用示例共享密码、过期身份材料或全局跨平面数据库身份；公开入口与证书 SHALL 与客户真实地址匹配。

**Traceability:** CFG-002, KERNEL-001, RT-002, P1-WRITE-003

#### Scenario: 身份材料不在有效窗口
- **GIVEN** 安装提供的身份材料已经过期或证书不匹配实际入口
- **WHEN** 工具校验安装前置条件
- **THEN** 安装返回具体问题并拒绝宣告平台可用
- **AND** 不继续使用过期或示例材料启动交付成功门

### Requirement: [PINST-004] 所有者隔离的数据库迁移
安装 SHALL 以数据库 owner 为边界执行版本固定、可校验和互斥的迁移，阻止不兼容 schema 下的平台服务就绪。恢复 SHALL 核验迁移记录和实际 schema，不得将资源安装回滚等价为数据库数据回滚；任何跨平面共享表或跨 owner 读写 SHALL 被拒绝。

**Traceability:** P1-WRITE-003, GOV-003, GOV-006, PINST-003

#### Scenario: 并发安装请求执行同一迁移
- **GIVEN** 一个 owner 的迁移已由另一个安装执行持有互斥身份
- **WHEN** 第二个安装尝试运行相同迁移
- **THEN** 第二个安装等待或以明确冲突失败
- **AND** 不重复或交错修改 schema

### Requirement: [PINST-005] 完整业务就绪和真实纳管交接
安装成功 SHALL 通过节点/集群、平台入口/身份、依赖持久化、Provider 路由和至少一次真实运行执行与 Agent 观测的各层必需断言。管理基座纳管 SHALL 使用正式注册、认证和观测契约；初始化记录 SHALL 仅作为关联附件，不得直接生成虚假 Operation 或成功 Read Model。

**Traceability:** GOV-004, RT-002, RT-008, RT-009, P1-WRITE-005, DPLY-005

#### Scenario: 平台组件运行但真实 Operation 未完成
- **GIVEN** 平台服务可登录但验证用 Release 的执行或 Agent 观测失败
- **WHEN** 安装器执行最终验收
- **THEN** 交付结果保持未完成并展示实际缺环
- **AND** 不把集群 Ready 或服务健康结果提升为完整交付成功

#### Scenario: 管理基座交接
- **GIVEN** 管理基座和平台已通过就绪检查并有真实初始化记录
- **WHEN** 执行纳管交接
- **THEN** 正式注册 Operation 和认证观测建立管理关系
- **AND** 初始化阶段时间不能被伪装成 Agent 实时观测时间

#### Scenario: 登记动作级 Provider 验证报告
- **GIVEN** 隔离验证产生受信签名者签署且绑定 provider 物料、基础 manifest、精确矩阵动作和有效期的报告
- **WHEN** 具备 provider execute 权限的调用者提交报告到平台 owning-plane
- **THEN** 平台验证签名及完整通过断言并在事务中重新核验 manifest 身份后登记该动作证据
- **AND** 未受信、失败、过期、身份不匹配或并发更换 manifest 的报告不能提升资格
- **AND** 动作级资格不提升为完整平台、HA 或未测试动作认证

#### Scenario: 重驱动丢失的步骤请求
- **GIVEN** 非终态 Operation 的步骤仍为 pending，且其步骤请求消息已投递后随消费者消失
- **WHEN** 清扫周期到达且该步骤空闲时间超过消费者 AckWait，并且不存在尚未发布的请求
- **THEN** worker 重新武装与步骤幂等键一致的既有 outbox 请求行，保留原始消息身份、幂等键与 payload，仅更新投递状态、计数与时间后重新发布
- **AND** 重投递不超过步骤重试预算，并发清扫不会对同一步骤重复投递
- **AND** 重复消息不会导致重复执行，租约与 fencing 仍只允许一个持有者
- **AND** 超过预算的步骤保持未完成并输出显式告警，不被静默终止

### Requirement: [PINST-006] 升级恢复与数据回退边界
平台 SHALL 为获支持的升级和恢复路线提供物料兼容差异、备份/恢复要求及真实演练证据。正常在线变更 SHALL 经 Operation；控制面不可用时的现场恢复 SHALL 仅限已绑定安装身份的 HNB 管理基座和备份，不得借此执行业务目标变更或覆盖 Operation 历史事实。

**Traceability:** KERNEL-002, GOV-006, OBS-004, DPLY-004, DPLY-006

#### Scenario: 新平台资源版本依赖前向迁移
- **GIVEN** 新版本的数据库迁移不可直接反向执行
- **WHEN** 平台升级后需要恢复
- **THEN** 运维流程明确区分资源版本恢复和数据前向恢复/备份恢复
- **AND** 不宣称单次资源回滚已恢复数据

### Requirement: [PINST-007] 基于所有权的卸载和资源保留
工具 SHALL 区分卸载 HNB、解除纳管和删除本次创建的管理基座，并按真实所有权清单及现有权限/执行策略操作。卸载 HNB SHALL NOT 隐式删除已有 Kubernetes、共享中间件、业务资源、外部存储或非本次安装拥有的持久数据。

**Traceability:** RT-009, KERNEL-002, GOV-006, PINST-001

#### Scenario: 卸载部署于已有 Kubernetes 的 HNB
- **GIVEN** 平台使用客户已有 Kubernetes、数据库和存储
- **WHEN** 用户请求卸载 HNB 平台
- **THEN** 只处理获授权且明确属于平台安装的资源
- **AND** 已有集群和客户业务数据保留
