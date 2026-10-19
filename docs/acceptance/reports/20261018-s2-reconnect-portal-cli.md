# S2 断连恢复、source-reset 与 Portal/CLI 复验

- run_id: `s2-reconnect-portal-cli-20261018`
- commit: `533d8ae41b386d9ae738528f73641720d40f997f` + 本批工作树增量
- scope: 独立业务目标 `ef14ba93-f2df-523f-9231-cd2fad333cc3` 的 Agent 停机/重入网、
  PVC 状态恢复、source-reset、STALE/FRESH、Portal BFF 与 CLI 列表。
- result: 3.4 通过；3.5 仅 Portal/CLI 同一目标列表通过，跨租户与迟到响应未完成。

## 3.4 断连与恢复

### Agent 进程重启（PVC 恢复）

- Agent state 从 seq 8 继续到 seq 9，无 gap、无重复 target。
- session/observer token/state 均在 `hnb-agent-state` PVC 上，PVC 绑定成功。

### Agent 停机超过 stale 窗口

- 停机：`03:55:27Z`，恢复启动：`04:01:40Z`（约 6m13s）。
- 停机期间目标：

```text
status=STALE freshness=STALE
lifecycle=ACTIVE health=HEALTHY connectivity=CONNECTED
observedAt=2026-10-18T03:51:30Z
available=false observationState=stale
observationGeneration=4 observationRevision=15
```

旧状态保留，只有 freshness/available 变为 stale。

### 重入网与 source-reset

- 新建 enrollment，session generation 5。
- PVC state 仍为 generation 4；Agent 启动后调用
  `POST /v1/observations/reset`，previous=4/new=5。
- 观测游标变为 generation 5 / revision 1，随后 revision 更新。
- 最终：

```text
status=RUNNING freshness=FRESH
runtimeVersion=v1.36.5+k3s1 nodeCount=2
available=true observationState=observed
observationGeneration=5 observationRevision=9（source-reset 后从 1 继续）
```

没有新建重复 target，没有通过直接改游标完成本次恢复。

## 3.5 Portal / CLI

同一 admin token：

- Portal BFF `GET /api/v1/resources/clusters` 返回
  `ef14ba93… / hnb-target-live3 / RUNNING / available=true`。
- `hnbctl cluster list --platform-url https://10.9.180.147:30443 --token-file …`
  返回同一目标，`RUNNING`、`ACTIVE=true`、K3s 版本一致。
- CLI 访问 HTTPS lab 入口时使用显式 CA 信任；未关闭 TLS 校验。
- `--tenant-scope other` 未扩权，仍只返回 token 所属 tenant 的目标；该参数目前未被
  CLI 发送，属于待修正的输入语义，不能作为正式跨租户拒绝证据。
- `hnbctl cluster import` 已补齐 `--display-name` 和 `--credential-*`，用相同
  idempotency key/correlation/credential ref 重放同一 import，返回
  intent `914b4bc5…`、operation `600fabde…`、`replayed=true`。
- CLI 的 `--target-kind kubernetes` 映射为 `KubernetesTarget`，避免 BFF 400。
- 迟到响应隔离仍未执行；跨租户需要第二个 tenant token，3.5 保持 partial。

## 本轮新增修复

1. Agent 上报失败时保留同一 payload/sequence，成功后只推进一次；解决断连后
   sequence gap。
2. 新增 observer state 文件（generation + nextSequence），原子写入，PVC 持久化。
3. Agent 启动检测 generation 提升并调用 source-reset，成功后落盘新 generation。
4. session store/state 允许 fsGroup 组读（仍拒绝 world-readable），适配 PVC。
5. `hnbctl cluster list` 契约从不存在 `/api/v1/targets` 改到 Portal BFF
   `/api/v1/resources/clusters`，并更新测试。

## 边界

- TLS 入口是 lab Python 反向代理，不是管理面正式 Gateway；仅用于 CLI 验证。
- seeded `cluster-a/cluster-b` 仍显示 STALE，不属于本轮目标。
- Provider conformance 是 1 小时 lab 报告，已过期后不能用于新 import。
- 服务 subject 仍为 lab 直接 provision；正式 IdP/Subject API 未完成。
- 3.3/3.4 可勾选；3.5 未完成。
