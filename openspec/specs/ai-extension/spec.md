# ai-extension

## Purpose
定义 AI Access、Runtime、Governance、Copilot 和 AIOps 的可选边界。

## Requirements

### Requirement: [AI-001] AI 平面可独立启停
AI Extension Plane SHALL 可独立安装、升级、停用和卸载；停用 SHALL 不影响 T0/T1 平台、普通 Gateway 和传统服务。

**Traceability:** AI-ARCH-01, AI-ARCH-02, AI-ARCH-05

#### Scenario: 停用 AI Access Pack
- **GIVEN** 平台存在传统应用
- **WHEN** AI Gateway 被停用
- **THEN** 传统应用交付和运行不受影响
- **AND** AI 菜单和入口按能力隐藏

### Requirement: [AI-002] 统一模型资源模型
AI 扩展 SHALL 提供 ModelArtifact、ModelService、ModelEndpoint、AIProvider、PromptTemplate、GuardrailPolicy、EvaluationSuite 和 AIUsageRecord，并固定版本、来源、许可证和评测状态。

**Traceability:** AI-RUN-01, AI-RUN-02, AI-RUN-03, AI-GOV-01

#### Scenario: 发布外部模型引用
- **GIVEN** 模型不存储在 HNB Registry
- **WHEN** 发布者登记 externalRef
- **THEN** 系统固定提供方版本、区域、策略和评测状态
- **AND** 调用前可验证租户授权

### Requirement: [AI-003] AI Gateway 流量治理
AI Gateway SHALL 支持 HTTP、SSE、WebSocket、OpenAI-compatible、路由、限流、重试、熔断、Fallback、安全围栏、脱敏、用量和成本；普通业务流量 SHALL NOT 经过该数据面。

**Traceability:** AI-GW-01, AI-GW-02, AI-GW-03, AI-GW-04, AI-GW-05

#### Scenario: 外部模型超时
- **GIVEN** 租户配置了可用 Fallback
- **WHEN** 模型请求超时
- **THEN** AI Gateway 按策略路由到备用模型
- **AND** 调用审计记录主备 Provider 和成本

### Requirement: [AI-004] Copilot 无执行旁路
Copilot 和 AIOps SHALL 输出证据、时间范围、影响对象、置信度和不确定性；任何写操作 SHALL 转换为结构化计划并经过权限、策略、审批和 Operation。

**Traceability:** AI-OPS-01, AI-OPS-02, AI-OPS-03

#### Scenario: Copilot 提议扩容
- **GIVEN** 诊断建议将副本从 2 扩到 4
- **WHEN** 用户确认建议
- **THEN** 系统生成可审计 Operation
- **AND** Copilot 不直接调用 kubectl

### Requirement: [AI-005] 高风险自动化限制
删除、数据库切换、灾备、网络、存储和大规模扩缩容 SHALL NOT 无确认自动执行；自动修复 SHALL 支持熔断、冷却、回滚和效果验证。

**Traceability:** AI-OPS-05, AI-OPS-06, AI-OPS-07

#### Scenario: 自动修复无改善
- **GIVEN** AIOps 已执行一次低风险修复
- **WHEN** 验证指标未改善
- **THEN** 系统停止连续动作并升级人工处理
- **AND** 保留失败证据
