# S0 1.3：三主机独立集群 / CSI / Gateway 基线

- run_id: `s2-s3-lab-20261018`
- commit: `533d8ae41b386d9ae738528f73641720d40f997f` + 本批工作树增量
- requirements: RTLOOP-001、DEVEX-002、OBSLOOP-002（基础设施准备范围）
- command: `python3 scripts/lab/verify-environment.py --output docs/acceptance/reports/20261018-s2-s3-lab-infrastructure.json`
- started_at: `2026-10-18T00:16:27Z`
- finished_at: `2026-10-18T00:16:47Z`
- exit_code: 0
- result: `20261018-s2-s3-lab-infrastructure.json`，8/8 断言通过，脚本/配置摘要见 JSON。

## 环境与执行

| SSH 主机 | 角色 / 内网 | 配置 | 最后采样内存 | 根分区可用 |
|---|---|---|---|---|
| 106.75.37.46 | 管理单节点 / 10.9.180.147 | 4C / 7936MiB / 20G | 771Mi | 9.6GiB |
| 106.75.15.35 | 业务控制节点 / 10.9.14.115 | 2C / 3913MiB / 20G | 866Mi | 11.4GiB |
| 106.75.25.31 | 业务 worker / 10.9.37.197 | 2C / 3913MiB / 20G | 792Mi | 12.1GiB |

Ubuntu 24.04 LTS / amd64 / Linux 6.8.0-138-generic；初始为空白节点，根盘约剩 17G。
以上内存为单次 `kubectl top nodes` 样本，不是平台容量或峰值保证。

1. 通过 SSH 确认 sudo、初始资源和服务；私网 ICMP 通过。
2. `bootstrap-k3s.sh server hnb-mgmt 10.9.180.147 none management`；
   `bootstrap-k3s.sh server hnb-target-control 10.9.14.115 none target`。
3. server node-token 经 SSH 管道 stdin 输入 worker，执行
   `bootstrap-k3s.sh agent hnb-target-worker 10.9.37.197 10.9.14.115 target`。
4. 官方同版本基础镜像包/二进制校验 release SHA256，安装器固定内容 SHA256。
   Docker Hub 不可直连，GitHub 与 ghcr.io 可达；不因此宣称离线认证。
5. `prepare-loop-storage.sh 5`（管理）/ `3`（业务各节点），预分配真实磁盘空间；
   loop PV 仅指向新建专属文件，没有把系统磁盘设备作为 LVM PV。
6. `helm template topolvm ... --include-crds -f values-minimal.yaml -f values-s2-s3-lab.yaml`
   经 `kubectl apply --server-side --field-manager=hnb-lab` 应用到两个集群。
7. 业务安装 Gateway API 标准 CRD、固定 Chart/镜像的 Traefik、开发自签 TLS Secret，
   应用 `deploy/lab/target-probes.yaml` 与 `probe-route.yaml`。
8. worker Pod 写新标记后删除/重建，再执行 worker `systemctl reboot`。
   `hnb-lab-loop` 与 `k3s-agent` 自动恢复，原 PVC 和标记保留。

## 固定身份和物料

- 管理 kube-system UID：`4257dade-ca50-47a8-bbdc-e287465b19ec`；
  业务：`fed7cd86-cc35-450f-a3a7-de6614578787`。
- K3s `v1.36.5+k3s1` / containerd `2.3.4-k3s1.36`。
- TopoLVM Chart 17.3.0，SHA256
  `d51eb34db2e676f21a6e5a814bc8cc7ed41cd0f6505fab92956f06155f2d65ac`；
  app 0.42.0 / 镜像 `sha256:719ab11d974af827d12ab148ca9c070babcb21980b8c7f30497c6bde81cdce4b`。
- SC `hnb-topolvm` / Retain / WFFC / expand；Snapshot 未启用。
- Gateway API `v1.6.3/standard-install.yaml`；Traefik 3.7.13 / 镜像
  `sha256:f86a2cab1b5c649070c49f883c743dd32d8485a56e3368c5f93b9e91f1e91259`。
- K3s 自带 Chart `40.1.5+up40.1.0`，SHA256
  `d4d0e818284d310372d38adfce9a1cfb3179a69fc9f3ca8bfadb33581000b778`。
  首次使用上游 40.1.0 因只支持至 3.7.1 被拒绝，改用匹配 K3s Chart，未绕过版本检查。
- Probe busybox：`sha256:101b4afd76732482eff9b95cae5f94bcf295e521fbec4e01b69c5421f3f3f3e5`。

## 通过断言及失败保留

- 两个独立 cluster UID，管理 1 节点、业务 2 节点均 Ready。
- 两个集群 controller 各 1、CSI node 分别 1/2，全容器 Ready。
- worker 的真实 CSI PVC Bound，UID `ccf0da63-240d-4481-bf26-7424e5273850`。
- 控制节点 Job 经 DNS/Service 读 worker 卷文件；首次后端未 Ready 导致连接拒绝，
  Job 重试通过，失败 Pod 保留可查。
- Pod 从 `aa5047ef-0fdb-4f26-8bd2-fbb3894ed4cf` 重建为
  `60641b0e-f327-47eb-8c2c-f8415983764a`，同 PVC 读回
  `hnb-persist-after-recreation-20261018`；worker 整机重启后也读回同值。
- Gateway Programmed=True；HTTPRoute Accepted=True / ResolvedRefs=True。
- 外部经 `106.75.15.35:80/443`、Host/SNI `app.hnb.test` 读同标记，均 HTTP 200。
  HTTPS 验证显式信任的开发证书，不关闭验签；无需公共 DNS，可用 `curl --resolve`。
- 三节点均至少剩 7GiB（本次继续部署的观察门，不是生产容量预算）。

TLS verifier 初次使用 IP SNI 命中默认证书，exit=1；保留
`20261018-s2-s3-lab-infrastructure-first-failure.md`，修正域名 SNI 后通过。

## 部署前修复：续期 wire response 和 Chart

真实 Renew handler 返回 credentialId/serviceIdentityId/subjectId/tenantId/audience/
permission/issuedAt/expiresIn 等元数据。原 HTTPCredentialIssuer 严格解码只允许两个
字段，拒绝真实成功响应。新增真实 handler→HTTP client 回归，修复前明确失败
`service credential unavailable: invalid renewal payload`，修复后两轮续期、权威验签和
撤销拒绝通过。修复结构认识既有字段，不放宽 scope/签名验证。

worker Chart 新增显式 serviceIdentity、公钥挂载、四个 SERVICE_IDENTITY_* 变量；
issuerURL/issuer/公钥引用缺失拒绝，legacy 路径兼容。

通过命令：

```sh
go test ./pkg/iam/... ./cmd/operation-worker/internal/config/... ./cmd/apiserver/internal/handler/...
go test -race ./pkg/iam/... ./cmd/apiserver/internal/handler/...
go vet ./pkg/iam/... ./cmd/apiserver/internal/handler/... ./cmd/operation-worker/internal/config/...
python3 deploy/charts/hnb/test_candidate_components.py  # 31 passed
python3 deploy/charts/hnb/test_profiles.py              # 37 passed
bash -n scripts/lab/bootstrap-k3s.sh scripts/lab/prepare-loop-storage.sh
```

| 源码/配置 | SHA256 |
|---|---|
| `pkg/iam/http_credential_issuer.go` | `7d9ac25c0c4c0798d21fd5180809bc857e27d59f66f9889b141799a40603ce0d` |
| `cmd/apiserver/internal/handler/service_identity_test.go` | `26f5c4977bb09a8f7d5f0cc3028deacbaa63780c3d75c0c14d9bf119926e0484` |
| `operation-worker/values.yaml` | `73191422a3c1ceeaa8a3bdb0ffedb0c123fe00dee109fdecf70c4fa9890b3596` |
| `operation-worker/templates/deployment.yaml` | `89d70d856bcfc904035a2d08a3ce317ccb031d0c25a5da864c630fa2518f9a29` |
| `test_candidate_components.py` | `2fb030d6ef18c8ed4ff9c65a787ae17c50d813b527438bf4b5281de967345fb2` |

## 边界 / 下一步

完成 1.3 环境基线及 ≥3 故障域需求登记。当前仅部署 K3s、CSI、业务 Gateway/probe；
管理面最新 HNB、PG/NATS、Portal 尚未部署。Probe 是直接 Kubernetes 基础设施探针，
不是 HNB import、Agent 入网、Provider Conformance 或 S3 黄金路径。

下一项为当前源码物料构建/管理面部署、正式服务身份及 Agent enrollment 部署接线，
再执行 3.3→3.4→3.5。20G root-backed loop 非生产存储；HA/容量/RTO/RPO、独立实施者
安装、完整发布和公共 DNS/公信证书均未由本报告认证。
