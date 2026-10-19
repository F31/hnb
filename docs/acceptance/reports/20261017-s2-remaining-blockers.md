# S2 3.3–3.5 剩余条件与本地证据

- Run ID：`s2-remaining-blockers-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：RTLOOP-001、UXLOOP-001、IDENT-001、IDENT-002
- 结论：3.1/3.2 已完成本地契约与真实 PostgreSQL 验证；3.3–3.5 仍需独立业务
  集群或真实 Portal/CLI 旅程，保持未完成。

## 3.3 独立业务集群正式 import（blocked）

缺少与 `hnb-minimal-test` 不同的业务集群身份、kubeconfig/Agent 授权、可调度资源
和固定 CSI/SC/Gateway。禁止用同集群自导入或直写投影替代。解除条件见
`docs/deployment/independent-target-baseline.md`。

## 3.4 Agent 断连/重连（partial）

已有本地证据：

- 2.8：跨 >2 TTL 自动续期、Agent 重启从 0600 session 恢复、撤销关闭 3.95ms。
- 3.2：重复/缺口/旧 generation/source reset 与跨租户/跨目标不更新投影。

仍缺：独立目标上真实 Agent 断连的时间线、旧状态保留、超阈值 STALE 提示、
恢复后的新鲜观测与目标不重复。2.8/3.2 不能替代该实机验收。

## 3.5 实际 Portal/CLI 纳管旅程（blocked）

Portal 同仓可用、CLI `hnbctl` 已有 handoff/操作入口，但未在独立目标上执行同一
纳管旅程；跨租户拒绝与迟到响应隔离只在 handler/projector 层验证，不等于 UI/CLI
实机验收。Playwright E2E 需要运行中的 BFF/Portal 与独立目标。

## 下一步

2026-10-18 更新：三主机已经建立独立管理/业务集群，CSI 与 Gateway/TLS 实测通过，
见 `20261018-s2-s3-lab-infrastructure.md`；原“无独立主机/目标”环境阻塞解除。
管理集群当前尚未部署最新源码 HNB，正式身份注册、Agent enrollment 部署接线及
CLI/API import 仍待实施。本文 2026-10-17 的结果是历史状态，不将基础设施探针视为
3.3–3.5 完成。

提供独立业务集群、固定 CSI/SC、Gateway/TLS/DNS 与故障域后，按 3.3→3.4→3.5 顺序
执行；在此之前 S3 的实机黄金路径（4.3–4.12）也保持 blocked。可继续推进不依赖
实机的契约/迁移/单元测试。
