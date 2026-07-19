# portal-experience

## Purpose
定义 Portal 基于已安装能力、用户权限和目标能力动态呈现的简单、标准、专家三层体验，以及可恢复场景向导行为。

## Requirements

### Requirement: [UX-001] 能力驱动界面
Portal SHALL 根据已安装 CapabilityPack、用户权限和 RuntimeTarget 能力动态显示菜单、表单和动作；未安装能力 SHALL 不出现空菜单或不可执行入口。

**Traceability:** METH-03

#### Scenario: 未安装 Edge Pack
- **GIVEN** 用户登录平台
- **WHEN** Portal 构建导航
- **THEN** 不显示边缘菜单和向导
- **AND** 核心页面无 Edge 依赖错误

### Requirement: [UX-002] 三层操作模式
Portal SHALL 提供简单、标准和专家模式；简单模式面向业务对象，标准模式暴露策略和生命周期，专家模式才允许查看底层资源。

**Traceability:** METH-04

#### Scenario: 新用户交付应用
- **GIVEN** 用户使用简单模式
- **WHEN** 完成发布和部署
- **THEN** 无需直接编辑 Kubernetes CRD
- **AND** 专家模式仍受权限和策略约束

### Requirement: [UX-003] 场景化向导
平台 SHALL 为应用发布、数据库创建、服务暴露、备份恢复和 Gateway 迁移提供可恢复向导；安装对应 CapabilityPack 后，平台 SHALL 为边缘节点纳管和 AI 接入提供可恢复向导；所有产生运行变更的向导 SHALL 在提交前展示 ExecutionPlan 摘要。

**Traceability:** GW-14, GOV-04

#### Scenario: 向导中途退出
- **GIVEN** 用户已填写部分部署参数
- **WHEN** 稍后重新进入
- **THEN** 草稿被恢复且未产生目标资源
- **AND** 最终提交生成 Operation
