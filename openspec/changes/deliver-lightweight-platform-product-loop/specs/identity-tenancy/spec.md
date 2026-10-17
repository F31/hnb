## ADDED Requirements

### Requirement: [IDENT-001] 服务身份持续生命周期
平台 SHALL 为服务调用提供最小权限、限定 tenant/subject/audience/action/target 的自动签发、提前续期、轮换及撤销机制，续期 SHALL 有界重试且不依赖人工 mint token、无限延长 TTL 或进程重启。身份过期、撤销或无法验证时 SHALL 停止受保护调用，不得回退为共享静态凭据或透传不匹配的用户身份；撤销 SHALL 在预先批准的传播上限内对新调用和持久连接生效，审计区分 actor 和 service subject 且不含凭据值。

**Traceability:** P1-ING-004, P1-ING-005, P1-ING-006, TENANT-008

#### Scenario: 执行跨越多个凭据有效期
- **GIVEN** 正式服务身份和批准的续期策略已经配置
- **WHEN** 服务持续运行超过两个初始凭据有效期并经历一次客户端重启
- **THEN** 受保护请求通过自动续期继续工作，无需手工 token 或扩大权限
- **AND** 旧身份只在批准的重叠窗口内有效

#### Scenario: 撤销已连接服务身份
- **GIVEN** 某服务身份仍有有效期且持有活动长连接
- **WHEN** 管理员撤销该身份并达到批准的传播上限
- **THEN** 新请求被拒绝且原长连接被关闭或重新鉴权后拒绝
- **AND** 其他未撤销身份仍正常工作

#### Scenario: 续期服务不可用
- **GIVEN** 身份客户端续期失败且原凭据已过期
- **WHEN** 客户端尝试继续受保护调用
- **THEN** 请求被拒绝并产生不含凭据的可诊断原因，不使用过期缓存放行

### Requirement: [IDENT-002] Agent 单次入网与安全重连
Agent 入网 SHALL 使用绑定租户、目标、预期身份及有效期的单次 bootstrap credential，经受信服务原子消费后交换可轮换身份；重放、跨目标/租户或过期交换 SHALL 被拒绝。Agent 重连/重新入网 SHALL 复用或受控更新已注册目标身份，并依据观测协议建立 generation，旧身份或旧 generation SHALL 被 fencing；凭据及私钥不得进入日志、计划或诊断包。

**Traceability:** TENANT-008, P1-ING-001, P1-ING-005, RT-002, RT-008

#### Scenario: 同一 bootstrap credential 并发交换
- **GIVEN** 一个未消费且绑定目标 A 的入网凭据
- **WHEN** 两个客户端并发尝试交换该凭据
- **THEN** 最多一次交换成功，其余请求被拒绝并留存脱敏审计
- **AND** 不能用该凭据为目标 B 建立身份

#### Scenario: Agent 断连后恢复
- **GIVEN** 目标已注册且 Agent 暂时断连
- **WHEN** Agent 通过有效身份重新连接或按策略重新入网
- **THEN** 平台恢复同一目标的观测，不重复创建目标或接受旧 generation 覆盖
