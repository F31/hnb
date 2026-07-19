# release-package

## Purpose
定义 Package 与 ReleaseManifest 的完整性、目标兼容性、传统软件容器化运行约束，以及发布撤销后的影响分析和处置行为。

## Requirements

### Requirement: [REL-001] ReleaseManifest 完整性
ReleaseManifest SHALL 包含 Package digest、参数 Schema、依赖、targetTypes、架构、资源下限、生命周期、升级路径、安全证明和支持声明。

**Traceability:** MKT-04, CMPOS-02, EDGE-06

#### Scenario: 缺失兼容性声明
- **GIVEN** 一个 Release 未声明 targetTypes
- **WHEN** 发布进入 stable 门禁
- **THEN** 门禁拒绝发布
- **AND** 返回缺失字段列表

### Requirement: [REL-002] JAR/WAR 容器化运行
JAR/WAR 可以作为 OCI Artifact 入库，但运行时 SHALL 由不可变 OCI 镜像或受控标准运行时容器承载；平台 SHALL NOT 使用 systemd 或裸 java 进程直接运行。

**Traceability:** CTN-03, CTN-05, ART-04

#### Scenario: 部署 WAR 应用
- **GIVEN** 市场中存在一个 WAR ArtifactRuntimePackage
- **WHEN** 用户部署到 KubernetesTarget
- **THEN** 平台生成运行时容器、只读制品挂载和健康检查
- **AND** 生产推荐路径固定最终镜像 digest

### Requirement: [REL-003] 多架构与目标兼容预检
声明支持 amd64、arm64、GPU/NPU 或 EdgeRuntimeTarget 的 Release SHALL 在发布和部署阶段分别验证镜像平台、资源、网络和运行时能力。

**Traceability:** CTN-06, EDGE-06

#### Scenario: 部署到 arm64 边缘节点
- **GIVEN** Release 声明支持 EdgeRuntimeTarget
- **WHEN** 镜像缺少 arm64 Manifest
- **THEN** 市场或平台预检拒绝该部署
- **AND** 错误信息指出不兼容的具体 Artifact

### Requirement: [REL-004] 撤销与影响分析
Release 撤销 SHALL 生成签名撤销 Artifact，平台 SHALL 识别受影响实例并按风险策略执行告警、隔离、升级或停止。

**Traceability:** MKT-07, ART-07, EDGE-10

#### Scenario: 高危版本被撤销
- **GIVEN** 一个 Release 在多个租户运行
- **WHEN** 安全管理员发布撤销
- **THEN** 平台展示受影响实例和处置状态
- **AND** 边缘离线站点重连后继续执行处置
