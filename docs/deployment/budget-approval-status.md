# 预算批准状态（S0 1.4）

状态：**S1 身份开发输入已采纳；生产/容量/恢复预算未批准**。
本文件不生成“已批准”预算，也不授权安装或认证。

## 1. 已采纳（仅开发/测试）

| 组 | 范围 | 状态 |
|---|---|---|
| S1-IDENTITY | delegation 30s/上限 60s；service token 60s；observer 5min/上限 10min；Agent session 5min/上限 10min；enrollment 15min；50% TTL 续期；重试 1/2/4s；撤销目标 5s/60s（未实测） | 已采纳为开发/测试输入（2026-10-17） |

依据：`docs/deployment/decision-baseline.md` 与
`docs/deployment/service-credential-renewal.md`。S1 的 5s/60s 仍是目标而非
生产实测结果。

## 2. 未批准（阻断对应认证）

| 组 | 待批准内容 | 阻断 |
|---|---|---|
| APP-OPERATIONS | 应用部署/升级/删除的验证超时、外部探针、卷读写、Operation/投影延迟 | S3/S4 |
| PRODUCTION-BUDGETS | 冷安装/就绪、API/Operation 延迟、四档资源/每目标增量、稳定性窗口 | S4/S5 |
| RECOVERY | PG/JetStream/完整恢复集的 RPO/RTO、业务卷排除/纳入 | S6 |
| CAPACITY | 控制节点/Agent/连接/采集器容量上限与负载窗口 | S5/S6 |

A8–A17 的候选值、单位、窗口与通过规则见决策基线；在批准方、有效期和
实际环境条件填入前，readiness 校验器不得把这些值当作 approved。

## 3. 批准所需的记录

每项预算至少记录：数值、单位、负载/故障条件、窗口、采样间隔、样本数、
通过规则、批准方、批准时间、有效期、绑定 profile/release/backing。
缺任一项即视为未批准；测量后补批不改变当时证据的有效性。
