# 产品闭环 S0：复用、缺口与实施边界

日期：2026-10-16。Change：`deliver-lightweight-platform-product-loop`。
Requirement：PREADY-001、PREADY-006、PREADY-007、UXLOOP-001、DEVEX-001。

本清单是源码/留存证据盘点，不是业务或生产验收报告。盘点时 HEAD 为
`5e7fc07edd3eb4c1d627ef4c463f9d527676cc93`，存在既有未提交修改，故 HEAD
不能单独标识工作树或当前运行镜像。没有重新连接历史主机或确认其可用性。

## 1. 复用与缺口

| 范围 / Requirement | 已有实现及证据 | 实际边界与缺口 | 实现 owner / 后续任务 |
|---|---|---|---|
| 安装与发布 / PREADY-005 | `cmd/hnbctl/internal/toolkit/{render,cluster_applier,lock}.go`；standardize 4.9/4.10、5.7；registry `hnbctl_install_execute_uninstall` | 真实 apply/rollout 验证使用 fixture Chart；不是正式 19 负载包的干净环境安装 | 原 change 4.10/5.7/8.1 交付；本 change S4 独立实施者复验 |
| 已有 Minimal / PREADY-001 | registry `existing_minimal_platform_delivery`；`reports/20261016-existing-minimal-e2e.md` | 19 负载、登录/读 API、隧道与滚动恢复；不是独立目标上有状态应用交付 | 复用平台运行证据；本 change S2/S3/S4 补业务断言 |
| 服务身份 / IDENT-001 | `pkg/iam/{token,service,key_store,delegation}.go`；worker 的 `iam.FileTokenSource` | access token 上限 60s；FileTokenSource 每次读取，不能凭此推导发行/续期；BFF 已有每请求 delegation signer，应复用而非重做 | `pkg/iam`、apiserver 身份入口、worker/Provider；本 change S1 |
| Agent / IDENT-002 | `pkg/iam/{agent_tunnel_token,observer_token}.go`；`cmd/cluster-agent/main.go`；`cmd/apiserver/internal/handler/agent_onboarding_loop_test.go` | 认证与连接已有基础，不等于一次性入网交换、自动轮换、长连接撤销的真实闭环 | IAM + Agent/tunnel owner；本 change S1/S2 |
| 正式 import / RTLOOP-001 | standardize 4.7/4.8/5.8/5.9；registry `runtime_target_import_operation`、`provider_conformance_registration` | 已勾选任务附带明确范围：自导入、手工短时 token，尚无独立目标的 Agent/读模型验收；旧 conformance 有时效 | lifecycle Provider + platform-api/worker；本 change S2，不重做 import 引擎 |
| 运行写入 / ADL-001 | `openspec/specs/kubernetes-runtime-provider/spec.md`；现有 Kubernetes Provider 和 CompositionRelease | 当前承诺 Deployment deploy/delete；PVC/Service/挂载引用/入口需显式动作与真实 Conformance，不能放开任意 YAML | 现有 Provider/存储/Gateway owning modules；本 change S3 |
| 升级/扩容 / PUPG-006 | toolkit `profilespec.go`、`upgrade_plan.go`、RunStore；registry `profile_upgrade_and_scaling` | 真实演练为 Minimal→Minimal 及 fixture，HA 升级未认证；当前 planner 主要使用预设，实际已装能力/覆写保留需补齐 | toolkit + 既有执行器；本 change 5.3–5.6/7.9 |
| HA / DEVEX-002 | `in-cluster-ha-backings` 1.x–4.x；`charts/cloudnative-pg`、PG/NATS 模板 | Operator 是薄封装，CRD 预装；5.4/5.5 尚无三故障域故障/容量证据；阶段 hook 不等于真实 dual-run/cutover | 原 change 5.4/5.5/6.2；本 change 7.4/7.5 消费适用报告 |
| 恢复 / OBSLOOP-002 | registry `existing_minimal_backup_restore`；PG 130 表和身份文件 round-trip | 不含 JetStream 消息面、权威制品字节、完整恢复集合和 RTO/RPO；更不代表业务卷恢复 | 原 change 5.6 + 本 change 7.6–7.8 |
| 扩展 / DEVEX-001 | extension-controller、plugin-worker、`web/packages/plugin-sdk`；registry `helm_plugin_lifecycle` | 历史 ConfigMap 冒烟，不是实际 UI/backend action 的完整升级/回退/卸载 | 现有扩展 controller/SDK；本 change S6 |
| 资源预算 / PREADY-004 | registry `startup_capacity_under_load`、`docs/deployment/minimal-footprint-audit.md` | Startup 读混合负载为 partial，成功样本分位数、同机压测和 PG 连接计数存在限制；不能作为四档生产容量 | 本 change 1.4/6.7/6.8，先批准条件，再测量 |

以上 registry 路径均相对 `docs/acceptance/`；任务归属不因本文转移。
原 change 勾选仅证明其附注范围，不能替代新 change 的更高层退出门。

## 2. Portal 已在当前仓库，不是缺失仓库

当前真实路径是 `web/`，不是文档中假定的独立前端仓库：

| 职责 | 源码/版本入口 | 本地验证入口 |
|---|---|---|
| Shell/会话/导航 | `web/shell`，`@hnb/shell` 0.1.0 | `pnpm --dir web --filter @hnb/shell typecheck` / `pnpm --dir web build` |
| 共享鉴权 API client | `web/packages/api-client/src/index.ts` | `pnpm --dir web exec vitest run packages/api-client/src/__tests__` |
| Schema/插件 | `web/packages/schema-engine`、`web/packages/plugin-sdk`、`web/plugins` | `pnpm --dir web test:run` |
| 浏览器旅程 | `web/e2e`、`web/playwright.config.ts` | 启动实际后端/前端后设置 `PLAYWRIGHT_BASE_URL`，运行 `pnpm --dir web test:e2e` |

源码版本必须与后端一起记录工作树/构建摘要；package 的 0.1.0 不能代替物料身份。
Playwright 不自动启动服务（`webServer: null`），测试入口存在不代表真实 E2E 已通过。
当前未发现 `.github/CODEOWNERS`，人员维护者仍待项目方指定；技术修改归属上述模块。

### 公开边界

- 浏览器/Shell/插件经共享 API client 访问 apiserver BFF。开发代理 `/api` 指向
  `localhost:8001`；这不是生产地址或真实可达性证据。
- `contracts/openapi/console/v1/openapi.yaml` 为浏览器契约，cluster/Operation
  查询与 navigation/schema 均复用该边界，不让浏览器直连 platform-api 或 Provider。
- BFF→platform-api 复用受众限定、保留 actor 的 delegation；领域服务自己授权。
- `hnbctl` 的在线业务命令复用正式 API/Intent/Operation；Day-0 的本地安装与恢复
  仍使用已有 ownership/receipt/run 边界，不用原始数据库写入替代 API。

## 3. 环境与决策阻塞

| 项目 | 当前可证明的信息 | 待补条件 / 影响 |
|---|---|---|
| 管理基座 | 最近报告为 Ubuntu 26.04 / amd64 / 单节点 K3s 1.36.5+k3s1 / TopoLVM 0.42.0 | 活性及精确运行摘要需重新观察；不冒称新演练 |
| 独立业务集群 | 旧报告中的自导入不能满足 RTLOOP-001 | 提供独立集群身份、授权接入方式、可调度资源和网络；阻断 S2 实机 |
| CSI / Gateway | 管理基座历史 TopoLVM 不等于业务目标已具备相同后端；Provider 不等于 controller | 固定业务 CSI/SC 与一种 Gateway 实现的真实版本、摘要、地址/TLS；阻断 1.3/S3 实机 |
| HA | 原 change 5.4/5.5 未执行 | 至少三实际故障域及预算；阻断生产 HA 范围，不阻断本地契约开发 |
| 数值预算 | 现有 token 最长 60s 是协议约束，不是续期/撤销 SLO 批准 | TTL/续期/撤销、应用超时、性能/资源与恢复预算需批准后冻结；1.4 未完成 |
| 参考应用 | 尚无已锁定 v1/v2/故障版本的实际 OCI 摘要 | 先交付证据契约/校验器；摘要和真实数据探针验收仍未完成 |

## 4. 执行顺序及证据纪律

1. 本批完成可独立推进的盘点、四档基线及本地阶段证据校验基础；不提升生产资格。
2. S0 身份/API 契约和数值条件明确后进入 S1 的发行/续期/撤销及入网实现。
3. 有独立目标后按 S2→S3→S4 复验；安装包与 HA 原任务继续在原 change 完成。
4. 校验器是离线内容/范围核对，不签发认证、不更新 EvidenceGate、不授权 install。
   正式发布仍须 owning-plane 审核和真实测试报告；任何 fixture 都只证明校验器自身。
