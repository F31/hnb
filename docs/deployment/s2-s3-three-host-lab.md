# S2/S3 三主机开发环境

状态：2026-10-18 两个 K3s 集群、CSI 与业务 Gateway/TLS 已部署并通过基础设施验证；
管理集群已用当前源码 13 个 digest 固定镜像部署 `startup` 组合，三平面迁移和真实
admin 登录通过。S2 独立目标 import/Agent 观测和 S3 产品验收仍未执行。
源码基线：`533d8ae`。本配置采用方案一，预算范围为低负载开发/功能测试。

| 角色 | 公网 SSH | 内网地址 | CPU / 实测内存 | 根分区可用 |
|---|---|---|---|---|
| 管理集群单节点 `hnb-mgmt` | 106.75.37.46 | 10.9.180.147 | 4 / 7936MiB | 约 17G |
| 业务控制节点 `hnb-target-control` | 106.75.15.35 | 10.9.14.115 | 2 / 3913MiB | 约 17G |
| 业务 worker `hnb-target-worker` | 106.75.25.31 | 10.9.37.197 | 2 / 3913MiB | 约 17G |

三台均为 Ubuntu 24.04 LTS / amd64、Linux 6.8.0-138-generic、20G 系统盘、无 swap。
`ubuntu` 具有免密 sudo；初检未发现 Kubernetes、Docker 或业务监听端口。
管理节点到两台业务节点及业务控制节点到 worker 的私网 ICMP 已通过。
凭据仅用于运行时接入，不写入库存、报告或版本库。

## 拓扑和物料

- 两个独立 K3s `v1.36.5+k3s1` 集群；业务控制节点允许调度。
- 管理集群 Pod/Service CIDR：`10.42.0.0/16` / `10.43.0.0/16`。
- 业务集群 Pod/Service CIDR：`10.44.0.0/16` / `10.45.0.0/16`。
- API 监听内网，Agent 使用业务控制节点内网 `6443`；跨集群 HNB 入口随后配置。
- 初始禁用 K3s 自动安装 Traefik；业务集群单独渲染固定版本 Gateway controller，
  使用 Gateway API 1.6.3、K3s 自带 Chart `40.1.5+up40.1.0`（SHA256
  `d4d0e818284d310372d38adfce9a1cfb3179a69fc9f3ca8bfadb33581000b778`）、
  Traefik `3.7.13`（镜像 `sha256:f86a2cab1b5c649070c49f883c743dd32d8485a56e3368c5f93b9e91f1e91259`）。
- 两个集群使用 TopoLVM Chart 17.3.0 / app 0.42.0，镜像
  `sha256:719ab11d974af827d12ab148ca9c070babcb21980b8c7f30497c6bde81cdce4b`，
  SC `hnb-topolvm`（Retain / WFFC / expand）。Snapshot 未启用。
- Docker Hub 在管理节点实测不可直连；GitHub release 下载可达，因此引导使用
  同版本官方 airgap images 并校验官方 SHA256。这只是交付手段，不代表离线认证。
- 安装脚本：`scripts/lab/bootstrap-k3s.sh`；官方安装器固定内容 SHA256，版本二进制
  和基础镜像包校验官方 release checksum；Agent token 通过 stdin 输入。

## 20G 磁盘约束

- 引导前要求至少 12GiB 可用；只在这些已授权的干净开发节点运行脚本。
- Kubelet 容器日志每文件 5Mi、保留 2 个；镜像 GC 高/低水位 75%/65%。
- 不在云主机编译源码；构建在开发机执行，上传固定摘要产物后清理临时 tar。
- 管理 PG/NATS PVC 各 1Gi（已安装并 Bound）；CSI、实际占用和可用空间必须
  先核验，不能用 requests 或稀疏镜像逻辑大小当真实剩余容量。
- 管理节点预分配 5GiB、业务各节点预分配 3GiB 的 root-backed loop 文件，VG
  `hnb-lab-vg`，每节点保留 1GiB spare；文件实际占用已计入根盘，非稀疏容量承诺。
  `hnb-lab-loop.service` 在 K3s 前激活；已重启业务 worker 验证重新挂载和数据保留。
  根分区接近 20% 剩余时暂停新物料/卷写入并检查占用。

## 当前可复查的基础设施

- 管理集群 kube-system UID：`4257dade-ca50-47a8-bbdc-e287465b19ec`。
- 业务集群 kube-system UID：`fed7cd86-cc35-450f-a3a7-de6614578787`。
- 业务 `hnb-gateway/hnb-lab`：Programmed=True；GatewayClass `hnb-traefik`。
- `hnb-lab-probes/probe-server` 位于 worker，挂载真实 256Mi CSI PVC；控制节点上
  Job 经 DNS/Service 读回数据。初次 Job 在后端就绪前 connection-refused，重试通过。
- 运行中探针写入 `hnb-persist-after-recreation-20261018` 后重建 Pod，并重启 worker；
  PVC UID 不变、标记保留。此探针通过直接 Kubernetes 基础设施操作部署，不是 HNB 应用。
- 外部 HTTP/TLS Host 为 `app.hnb.test`；不依赖公共 DNS，使用 `curl --resolve`。
  业务控制节点 `80/443` 公网已验证。TLS 为 30 天开发自签证书；私钥在控制节点
  `/var/lib/hnb-lab/tls/probe.key`（0600），公钥证书 `probe.crt` 用于显式信任验证。
- 域名/SNI 与 HTTP Host 都必须为 `app.hnb.test`；直接 IP SNI 会命中默认自签证书。
- 可复查：`SSHPASS=... python3 scripts/lab/verify-environment.py --output <report.json>`；
  凭据不写入报告。支持 SSH keys，不使用密码时省略 SSHPASS。

## 管理面当前状态

- `hnb-system` 10 个工作负载 Ready：apiserver、platform-api、app-market、
  operation-worker、extension-controller、cluster-router、tunnel-server、portal、
  postgresql、nats；镜像全部 `@sha256` 固定。
- 三平面（platform/market/artifact）各应用 104 个迁移，ledger 最大版本 105。
- Portal ClusterIP 经本机 port-forward 验证：admin 登录 200，workspaces/tenants/agents
  200，匿名 workspaces 401。
- 修复了 Go 1.26 拒绝 `{credentialId}:revoke` 导致的 apiserver 启动 panic；现路径为
  `/credentials/{credentialId}/revoke`，并有路由注册测试。
- lifecycle Provider 与 operation-worker 已接线并 Ready；service identity、provider
  token Secret 和 target kubeconfig SecretReference 已建立。
- observer token 已绑定 Agent session 的 tenant/target/generation，并随 exchange/renew
  轮换；Agent 写入私有 observer token 文件。详见
  `docs/acceptance/reports/20261018-observer-session-binding.md`。
- Provider manifest 当前 `conformance_level=none`；platform-api 要求
  `production_ready` + 精确 matrix cell 的签名 conformance 证据，因此 import 返回
  `503 PROVIDER_INCOMPATIBLE`。没有关闭校验、伪造 evidence 或直写 runtime_targets。
- 正式服务 subject provision API 仍缺失；本轮开发库显式 provision 了一个独立
  `identity_subjects` 行（`issuer=lab`）用于身份引导，不能视为生产 IdP 流程。
- 业务集群已部署 Agent；正式 API import、幂等重放、enrollment/session/tunnel 和
  2 节点 FRESH 观测通过。目标 `ef14ba93-f2df-523f-9231-cd2fad333cc3` 当前
  ACTIVE/HEALTHY/CONNECTED/FRESH、available=true。
- 本轮修复 tunnel-server session 校验、tenant bypass、freshness/capability 投影。
- Agent state/session/observer token 已放到 PVC；generation 5 通过 source-reset 建立，
  未再直接改游标。3.3/3.4 已通过。
- Portal BFF 与 `hnbctl cluster list` 已用同一 token 看到同一目标；3.5 仍 partial，
  缺第二 tenant token 的跨租户拒绝、迟到响应隔离和 Portal/CLI 同一 import 动作。
- 证据：`docs/acceptance/reports/20261018-management-deployment.md`、
  `20261018-observer-session-binding.md`、`20261018-s2-import-agent-observation.md`
  及同名 `.json`。

## 推进顺序和退出条件

1. 两个集群基础健康，记录各自 `kube-system` UID 与节点身份，验证真实跨节点网络。
2. 管理集群部署当前源码构建的 HNB，验证迁移、登录及服务身份接线。
3. 业务目标通过正式 CLI/API import、Agent 入网和观测，执行 3.3–3.5。
4. 固定业务 CSI/SC、Gateway/TLS 与参考应用 digest 后执行 S3。

该环境不具备三控制节点/三故障域 HA；生产预算、容量认证及完整恢复保持原门禁。
1.3/3.3–3.5 不因主机可登录或 K3s Ready 自动完成。
