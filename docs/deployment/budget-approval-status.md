# 预算批准状态（S0 1.4）

状态：**S1 身份开发输入已采纳；APP-OPERATIONS（A8–A11）已于 2026-11-11 批准；生产/容量/恢复预算未批准**。
本文件不生成“已批准”预算，也不授权安装或认证。

## 1. 已采纳（仅开发/测试）

| 组 | 范围 | 状态 |
|---|---|---|
| S1-IDENTITY | delegation 30s/上限 60s；service token 60s；observer 5min/上限 10min；Agent session 5min/上限 10min；enrollment 15min；50% TTL 续期；重试 1/2/4s；撤销目标 5s/60s（未实测） | 已采纳为开发/测试输入（2026-10-17） |

依据：`docs/deployment/decision-baseline.md` 与
`docs/deployment/service-credential-renewal.md`。S1 的 5s/60s 仍是目标而非
生产实测结果。

## 1.1 APP-OPERATIONS（A8–A11）已批准

| 项 | 数值与单位 | 负载/窗口/采样 | 通过规则 |
|---|---|---|---|
| A8 `app.verify-deadline` | 总超时 **600s**；资源 300s / 观测 120s / 外部 180s，子预算可并行 | 每次 revision 验证；至少 **10 次** | 每次均在 600s 内完成，或返回明确失败；不以 HTTP 202/Operation succeeded 代替可用 |
| A9 `observation.freshness` | 资源观测 age **≤60s** 为 fresh，>60s 为 stale；无认证心跳 **≥90s** 标记不可达；陈旧 **≥300s** 告警 | 每 **10s** 检查，连续 **30min**，注入断连 | 过期观测不因收到心跳变 fresh；时间来源不可信记 Unknown |
| A10 `app.external-probe` | 间隔 **10s**，单次超时 **3s**；**2** 次连续成功确认可用，**3** 次连续失败确认降级 | 从业务集群外探测，覆盖正确版本/内容 | 不能只看 HTTP 200；原始失败仍计入统计，不改写 Kubernetes readiness |
| A11 `app.volume-probe` | **≤30s** 完成小数据写入、flush/fsync、回读与标记比对 | 仅写参考应用授权目录；唯一标记；至少 **10 次** | 不以挂载成功代替数据恢复；不在客户卷任意覆盖文件 |

- 批准方：项目用户（jinfeng105）
- 批准时间：2026-11-11
- 有效期至：2027-02-09
- 依据：`decision-baseline.md` 修订 0.1 §4（A8–A11）
- 适用范围：S3/S4 应用交付验证；不提升生产容量、HA 或恢复认证。
- 说明：A14 `operation.dispatch`/`projection.lag` 原与 APP-OPERATIONS 同组，但本批未批准，
  继续按 pending 处理。

## 2. 未批准（阻断对应认证）

| 组 | 待批准内容 | 阻断 |
|---|---|---|
| PRODUCTION-BUDGETS | 冷安装/就绪、API/Operation 延迟、四档资源/每目标增量、稳定性窗口 | S4/S5 |
| RECOVERY | PG/JetStream/完整恢复集的 RPO/RTO、业务卷排除/纳入 | S6 |
| CAPACITY | 控制节点/Agent/连接/采集器容量上限与负载窗口 | S5/S6 |

A12–A17 的候选值、单位、窗口与通过规则见决策基线；在批准方、有效期和
实际环境条件填入前，readiness 校验器不得把这些值当作 approved。

## 3. 批准所需的记录

每项预算至少记录：数值、单位、负载/故障条件、窗口、采样间隔、样本数、
通过规则、批准方、批准时间、有效期、绑定 profile/release/backing。
缺任一项即视为未批准；测量后补批不改变当时证据的有效性。
