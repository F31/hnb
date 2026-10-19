# S2 前置：管理面当前源码部署与真实登录

- run_id: `management-deployment-20261018`
- commit: `533d8ae41b386d9ae738528f73641720d40f997f` + 本批工作树增量
- requirements: PREADY-001、RTLOOP-001（仅部署前置，不完成 3.3）
- command: `/home/ubuntu/deploy-management.sh /home/ubuntu/management-full.yaml full`
- result: `20261018-management-deployment.json`，全部部署断言通过
- scope: 三主机 lab 的管理集群 `startup` 组合；独立业务集群尚未 import，S2/S3 未完成
- follow-up: Observer 会话绑定和 Provider 接线见 `20261018-observer-session-binding.md`；
  本报告表中的 apiserver digest 是首次部署快照，后续修复镜像另记。

## 构建物料

- 构建在开发机完成，不占用 20G 云主机编译空间。
- 13 个镜像由当前 checkout 构建或从既有本地镜像取得，全部以 `@sha256:` 固定。
- Portal 先用 `pnpm -C web build:all` 重建 Shell 和全部插件，再替换既有
  `hnb-eval/portal:20261013` 的静态资源；基础运行时摘要记录在 build manifest。
- `build-manifest.json` 保存在本机 `/tmp/opencode/hnb-lab-build/`，不提交私密凭据。
- 镜像经本机 `docker save` + zstd 流式导入管理节点 K3s containerd，并显式生成
  digest reference；未依赖 Docker Hub 直连或 tag 漂移。

关键镜像：

| 组件 | digest |
|---|---|
| apiserver | `sha256:16e049e63508370510567dbe04cafc3578f562fe2aa90ae83a828151b6f117a6` |
| platform-api | `sha256:6eb07c559629a4885d0168e80c755603deb423a730458691f6906e94b50cf163` |
| operation-worker | `sha256:fdf3ece464ce6986980b877d8366379d41e5c1934e490f497f2228b9cfe64238` |
| extension-controller | `sha256:c3ef76959fca5f566ed98139e5b97df4496ea0954b2be1a0a0fb31e9d9e527ac` |
| app-market | `sha256:8c2ed0ead07672b35d646376669e1492c5f02a17784f492368f4e95a701e7bfa` |
| cluster-router | `sha256:a404eecefeb2cce43bdd57fff4ae48df4c50034696122527931e4fdb17416563` |
| tunnel-server | `sha256:8086013a8cce6ff95b2d4c7c7ca7ae3f9577a3336a1acc85594e829ca463c7ca` |
| portal | `sha256:ef05e4d073b118530d0cea9d50239a7ab497a17f79baaf2486c67c0b28052048` |
| PostgreSQL 16 | `sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d` |
| NATS 2.11.1 | `sha256:4bb7463f82c83da51fd098d9ddbc0dccfdc7415e110b02de375f60066b392878` |

## 数据库和密钥

- 管理集群命名空间 `hnb-system`；PG/NATS 各 1 个实例，PVC 各 1Gi。
- 三平面迁移用共享 `pkg/db.Migrator` 在 `postgresql-0` 内执行：
  - `platform` / `market` / `artifact` 各 `applied=104`；
  - schema fingerprint `60d3d34fe602eb588e8d6f2508cc552f8fb3555b93a0de23421cc24819808483`；
  - ledger 最大版本 105。
- 凭据由部署脚本在管理节点生成并直接写入 Secret，未进入仓库、日志或本报告：
  PG admin/三平面、NATS 三平面、NATS TLS CA/server、身份签名密钥/公钥
  manifest、KMS master key、stale challenge、bootstrap admin。
- 数据库 DSN 使用 Chart 实际服务名 `postgresql`；首次生成的 `postgres` 是错误服务名，
  已在部署中发现并修正，未以 IP 写死或绕过 DNS。

## 运行结果

10 个工作负载 Ready：apiserver、platform-api、app-market、operation-worker、
extension-controller、cluster-router、tunnel-server、portal、postgresql、nats。

Portal ClusterIP 经本机 port-forward 验证：

```text
login                       200
/api/v1/workspaces          200
/api/v1/tenants             200
/api/v1/agents              200
anonymous /workspaces       401
```

未输出 admin 密码、token 或 Secret 值。

## 部署中修复的阻断

1. **apiserver 启动 panic**：Go 1.26 的 ServeMux 拒绝
   `/service-identities/{id}/credentials/{credentialId}:revoke`（通配符后接字面量）。
   改为 `/credentials/{credentialId}/revoke`，同步授权矩阵和文档。
2. 新增 `TestServiceIdentityRoutesRegisterAndMatchRevoke`：修复前路由注册 panic，
   修复后可匹配并返回 204；`go test ./cmd/apiserver/...`、相关 middleware/router
   tests 和 `go vet` 通过。
3. 首轮 kubectl apply 未带 namespace，资源落入 `default`；这些是刚创建的空 PG/NATS
   资源，已删除并在 `hnb-system` 重新应用。部署脚本已固定 `kubectl -n`。
4. 镜像导入只生成 tag reference，首次 Pod 尝试从 Docker Hub 拉取 digest 失败；
   已用 `ctr images tag` 生成 `repo@sha256` reference，Pod 从本地内容启动。
5. 迁移二进制在宿主无法解析集群 DNS，改为在 `postgresql-0` 内执行，未将凭据写入
   命令行或报告。

## 边界 / 下一步

管理面部署是 S2/S3 的前置，不等于独立目标纳管。尚未：

- 部署/启用 `runtime-target-lifecycle-provider`、`kubernetes-provider`；
- 注册 target kubeconfig SecretReference；
- 配置 operation-worker 的正式 service identity（当前为 `{}`，无 provider）；
- 提供正式的 observer token 签发/续期 API（当前仅存在 `//go:build ignore` 的
  临时签发工具，不能作为 Agent 观测验收）；
- 在业务集群安装 Agent 并执行 import/重放/断连重连；
- S3 应用、Portal/CLI E2E、HA 或生产声明。

因此 3.3–3.5 保持未完成；下一步应先补齐 Observer 身份签发/续期和 target 凭据
接线，不能用临时手签 token、同集群自导入或直接写投影替代。
