## ADDED Requirements

### Requirement: [DEVEX-001] 可独立复现的参考扩展
平台 SHALL 提供版本化开发模板、公共契约/SDK、可本地运行的工具和参考扩展，覆盖能力与权限声明、受控 UI 注册、真实后端 action、资源预算、健康、安装/启停/升级/回退/卸载及诊断。参考扩展 SHALL 在不修改 Core/Shell 核心代码的条件下通过完整生命周期；仅创建占位 ConfigMap 或本地假页面 SHALL NOT 作为完成证据。

**Traceability:** PROV-001, PROV-003, PROV-006, PROV-007, PROV-008, PROV-009, UX-008

#### Scenario: 开发者安装并回退参考扩展
- **GIVEN** 开发者仅持有 SDK、模板、文档及兼容的 HNB 实例
- **WHEN** 其构建扩展、执行真实 action，并注入新版本健康失败后回退
- **THEN** 旧版本和能力/导航快照恢复，权限隔离及审计仍有效，Core/Shell 无需改动

### Requirement: [DEVEX-002] 真实集成的逐动作支持矩阵
平台 SHALL 对首个交付链所需的 Kubernetes 生命周期、工作负载、一个实际网络/入口实现和一个真实存储后端建立逐动作、版本、物料摘要、环境与有效期的支持矩阵，包含契约、功能、安全、故障、幂等/fencing 和性能证据。新能力在证据通过前 SHALL 保持 unsupported/blocked；框架 fixture、已有 import 或其他后端通过 SHALL NOT 自动认证新增动作/后端。生产执行前 SHALL 验证实际依赖控制器/CRD、目标事实及有效认证，不能仅依赖用户声明就绪。

**Traceability:** PROV-004, PROV-005, RT-004, KRP-001, STO-003, ADL-001

#### Scenario: 存储框架测试通过但没有实际驱动
- **GIVEN** 存储契约 fixture 通过，但目标没有可用且已认证的实际 CSI 后端
- **WHEN** 用户请求参考应用的持久卷动作
- **THEN** 平台返回不支持或依赖缺失原因，不按 fixture 将该动作标记生产可用

#### Scenario: 用户声明 Operator 就绪但实际缺失
- **GIVEN** 安装参数声称 HA Operator 已就绪，但实际集群缺少必需 CRD 或健康控制器
- **WHEN** 系统进入依赖该 Operator 的副作用阶段
- **THEN** 实际预检阻断执行，声明值不成为放行依据
