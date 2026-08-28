## ADDED Requirements

### Requirement: [OP-008] 单调外部执行 fencing
平台 SHALL 为每次成功的 Step Lease 获取分配全局唯一且单调递增的 fencing generation，并 SHALL 在 Lease 释放后保留该 Step 已授予的最大 generation。Worker 的续租、重试和结果提交 MUST 同时匹配当前 attempt identity、generation、owner 和有效 Lease。

**Traceability:** OP-004, OP-007, RDI-005

#### Scenario: Provider 成功后 Worker 崩溃
- **GIVEN** Worker A 以 generation 41 创建了外部资源但尚未提交 Step 结果
- **WHEN** Lease 过期且 Worker B 获得更高 generation
- **THEN** Worker B 安全接管同一资源并提交一次成功结果
- **AND** Worker A 的延迟外部写入和数据库提交均被拒绝

#### Scenario: Lease 释放后重新获取
- **GIVEN** 一个 Step 的 Lease 已释放或删除
- **WHEN** 该 Step 再次获取 Lease
- **THEN** 新 generation 大于此前已授予的 generation
- **AND** generation 不因事务回滚、冲突或进程重启而复用
