## ADDED Requirements

### Requirement: [DPLY-001] 统一部署配置与能力选择
部署工具 SHALL 提供版本化配置初始化、严格校验与规范化结果，覆盖模式、档位、目标引用、主机角色/地址、凭据引用、网络、存储、真实平台入口和锁定物料。工具 SHALL 拒绝未知字段、无效角色、重复节点、重叠 CIDR、凭据内联及未获认证的配置组合；CLI 与 Portal SHALL 使用同一能力及配置契约。

**Traceability:** GOV-002, GOV-003, RT-004, RT-010, CFG-002

#### Scenario: 创建可填写的配置
- **GIVEN** 实施人员选择一个工具已支持的档位
- **WHEN** 执行配置初始化
- **THEN** 工具产生结构化配置并指出必填的实际入口、存储、凭据和发布锁
- **AND** 缺失必填值的配置不得执行安装

#### Scenario: 配置请求未支持的多控制面
- **GIVEN** 已选执行路线只有单控制面认证证据
- **WHEN** 配置声明三个控制面节点
- **THEN** 工具在节点变更前返回明确的不支持原因
- **AND** Portal 不将该组合展示为已可用能力

### Requirement: [DPLY-002] 无副作用的实施预检
工具 SHALL 对身份权限、OS/架构/内核、资源、磁盘、时间、网络/地址、集群所有权、CRI/CNI、存储、入口/TLS 和锁定物料可达性输出带实际观测及修复建议的预检报告。预检 SHALL NOT 安装软件、修改宿主机策略、向节点导入镜像、创建集群资源或提交执行 Operation。

**Traceability:** RT-004, RT-005, GOV-003, KERNEL-002

#### Scenario: 节点缺少安装前置工具
- **GIVEN** 节点缺少执行路线要求的工具
- **WHEN** 实施人员执行预检
- **THEN** 结果指出节点、缺项、修复说明和阻塞级别
- **AND** 工具不会自动在该节点安装缺失工具

#### Scenario: 公网 SSH 可达但目标 API 路径不通
- **GIVEN** SSH 路径可达且所选 worker/Provider 到 API 的路径不可达
- **WHEN** 执行相应网络预检
- **THEN** API 路径检查返回 blocked
- **AND** SSH 成功不得覆盖该结果

### Requirement: [DPLY-003] 显式物料准备与下载进度
工具 SHALL 将物料下载、校验及获授权的预加载作为显式准备阶段，按已固定物料身份显示源、进度、重试及失败原因。准备阶段 SHALL NOT 改变集群成员关系；托管 RuntimeTarget 上的写入 SHALL 经正式 Operation，不能从预检隐式执行。

**Traceability:** GOV-003, SEC-001, KERNEL-002, P1-WRITE-002

#### Scenario: 冷镜像下载需要重试
- **GIVEN** 安装需要下载锁定的 CNI 镜像且下载发生可重试网络错误
- **WHEN** 执行准备阶段
- **THEN** 工具展示真实下载及重试进度
- **AND** 镜像准备未完成不得被报告为集群已就绪

#### Scenario: 显式镜像源准备
- **GIVEN** 实施人员选择 mirror 物料模式
- **WHEN** 执行本地准备但未声明 registry origin 映射
- **THEN** 工具在下载与缓存创建前拒绝，且不得回退原始源
- **AND** 显式映射只改变 registry origin，物料摘要仍须逐项匹配发布锁

#### Scenario: 准备中断后重试
- **GIVEN** 部分物料已验证而后续下载失败或取消
- **WHEN** 工具输出准备结果并以同一发布锁重试
- **THEN** 失败结果包含发布锁摘要、起止时间和已验证物料与字节，并返回非零退出码
- **AND** 重试复验已完成缓存，仅重新下载未完成对象，进度保留实际来源、时间、次数与失败原因
- **AND** 已验证本地物料不代表安装或节点 preload 已获授权

### Requirement: [DPLY-004] 固定身份的恢复与单写者
恢复 SHALL 绑定原配置、锁定物料、目标/管理基座身份和已完成阶段的证据，重新验证前置/后置条件后才能继续。已提交在线计划 SHALL 不可变；工具 SHALL NOT 自动重置已初始化集群、在恢复时更换生命周期 owner 或创建重复执行。并发写者 SHALL 被租约/ownership/fencing 拒绝。

**Traceability:** P1-WRITE-002, P1-WRITE-005, RT-009, GOV-006

#### Scenario: 安装器在 CNI 之后中断
- **GIVEN** 管理基座初始化完成但后续平台安装中断
- **WHEN** 使用相同配置与物料恢复
- **THEN** 工具重新核验已有集群并从可恢复阶段继续
- **AND** 不重复初始化集群或隐式删除已有状态

#### Scenario: 恢复时替换物料锁
- **GIVEN** 原执行绑定物料锁 A
- **WHEN** 恢复请求提供不同物料锁 B
- **THEN** 工具拒绝将 B 注入原执行并提示生成新计划
- **AND** 原执行事实保留

### Requirement: [DPLY-005] 分层结果与可定位进度
工具 SHALL 输出 runId、阶段、节点、物料身份、真实耗时、原因码、恢复建议及退出码，并明确区分节点准备、集群就绪、平台就绪和纳管完成。系统 SHALL NOT 从 Pod Running、Chart 安装成功或 API 进程启动推导完整交付成功。

**Traceability:** GOV-004, GOV-005, RT-008, OBS-004

#### Scenario: 集群就绪但平台登录失败
- **GIVEN** 集群 readiness 通过而平台认证流程失败
- **WHEN** 工具生成安装结果
- **THEN** 结果标识集群已就绪、平台未验收及实际失败原因
- **AND** 返回非成功交付退出码

### Requirement: [DPLY-006] 在线 CLI 服从规范执行链
平台在线模式下的 CLI 集群创建、纳管、节点变更、平台升级、回滚、恢复和卸载 SHALL 使用服务端支持的 typed RuntimeIntent、固定 ExecutionPlan 与持久化 Operation。客户端 SHALL NOT 直接执行 Provider、写平台数据库、发送任意 steps/commands 或因服务不可达回退到本地 SSH 执行。

**Traceability:** KERNEL-002, P1-WRITE-001, P1-WRITE-004, RT-009, RT-010

#### Scenario: CLI 创建业务集群
- **GIVEN** 一个已认证租户和服务端支持的集群创建配置
- **WHEN** 用户通过 CLI 请求创建
- **THEN** 请求提交到正式执行入口并关联真实 Intent/Plan/Operation
- **AND** CLI 从服务端事实展示状态，不自行装机

#### Scenario: 平台暂时不可用
- **GIVEN** CLI 处于在线模式且平台 API 超时
- **WHEN** 用户请求集群变更
- **THEN** 工具报告连接失败并保留已知 Operation 引用
- **AND** 不触发 bootstrap 或本地目标写入

### Requirement: [DPLY-007] 脱敏诊断和实施操作卡
交付工具 SHALL 导出包含配置/物料摘要、预检、阶段、目标观测、服务日志、失败定位和实施操作卡的诊断包。工具 SHALL 对账号密码、私钥、kubeconfig、服务 token、敏感连接串及客户数据执行脱敏，并输出诊断包身份与采集时间。

**Traceability:** CFG-002, GOV-005, RT-009, OBS-004

#### Scenario: 导出失败部署的诊断包
- **GIVEN** 安装失败且日志中可能出现敏感认证字段
- **WHEN** 用户导出诊断包
- **THEN** 包含足够定位阶段与节点的非敏感证据
- **AND** 明文凭据不进入导出物或普通终端输出

### Requirement: [DPLY-008] 明确安装工具能力与自动化接口
安装工具 SHALL 提供版本、支持矩阵、命令帮助、结构化输出及非交互执行方式。尚未实现或未认证的命令 SHALL 返回明确原因和非零退出码，文档 SHALL 标记其实施阶段，SHALL NOT 通过伪造成功或无操作步骤掩盖能力缺失。

**Traceability:** GOV-001, GOV-004, RT-010

#### Scenario: 请求尚未认证的节点变更
- **GIVEN** 服务端尚未支持所选目标的节点增加动作
- **WHEN** 用户调用该动作
- **THEN** 工具返回服务端不支持原因
- **AND** 不执行脚本或返回安装成功
