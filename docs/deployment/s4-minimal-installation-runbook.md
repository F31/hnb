# S4 Minimal 独立安装操作卡（release-lock/v1 候选）

本卡面向**独立实施者**：只凭发布物料与本卡，在干净环境完成 Minimal（existing 集群 + bundled
数据面）安装。任何一步失败都必须停止并回退，不允许跳过校验。

## 0. 认证边界（只在此矩阵内有效）

- 操作系统：ubuntu-24.04、rocky-10.1（amd64）
- Kubernetes：1.36.4（kind v1.36.4 已验证；existing 集群路线。K3s v1.36.5+k3s1 兼容声明沿用，本候选未重新验证）
- 形态：Minimal / bundled（PostgreSQL、NATS 随 Chart 单实例部署）
- 不在边界内：bootstrap 裸机建集群、in-cluster-ha/external 数据面、LiteHA 及以上形态、离线包

## 1. 发布物料清单

| 物料 | 校验 |
| --- | --- |
| `hnbctl`（linux/amd64） | `release.lock.yaml` 中 `cli[0].digest`，`sha256sum` 对比 |
| `hnb-0.1.0.tgz`（Chart 包） | `charts[0].digest` |
| `release.lock.yaml` | 20 个镜像 manifest digest、3×107 条迁移 checksum |
| `migrations/<owner>/*.sql` | 每条 checksum 与 lock 一致 |
| `sbom/*.cdx.json` + `sbom/index.yaml` | 每个 Go 二进制的依赖清单与 binarySHA256 |

镜像全部来自发布 registry（本候选为 `harbor.hnb.local/hnb`），渲染产物只允许出现
lock 内的 `repository@manifestDigest`；出现任何 tag 引用即失败。

## 2. 前置条件

1. 一个空 Kubernetes 1.36.5 集群（无 hnb-system 命名空间），有可用的默认 StorageClass。
2. 节点能解析并信任发布 registry（自签 CA 时需把 CA 注入节点容器运行时）。
3. 操作机安装 `kubectl`、`helm`、本发布锁定的 `hnbctl`。
4. 集群管理员 kubeconfig（仅安装期使用）。

## 3. 一次性 Secret 准备（命名空间 `hnb-system`）

所有口令用 `openssl rand -hex 24` 生成；私钥/证书操作在加固目录（`umask 077`）进行。
Secret 只进集群，不进 values、不进仓库。

```bash
NS=hnb-system
kubectl create namespace $NS
# NATS 服务 TLS（tls.crt/tls.key/ca.crt；SAN 需覆盖 nats、nats-headless、*.hnb-system.svc...）
kubectl -n $NS create secret generic hnb-nats-tls \
  --from-file=tls.crt=nats.crt --from-file=tls.key=nats.key --from-file=ca.crt=ca.crt
# 平台签名密钥：EC P-256；manifest.json 记录 issuer/generation/activeKeyId/keys
kubectl -n $NS create secret generic hnb-identity-keys \
  --from-file=manifest.json=manifest.json --from-file=public.pem=public.pem
kubectl -n $NS create secret generic hnb-identity-private --from-file=tls.key=private.pem
# PostgreSQL：admin + 三个平面（password 与 dsn 同密；dsn 主机名为 postgresql）
kubectl -n $NS create secret generic hnb-pg-admin --from-literal=password="$ADMIN"
for p in platform market artifact; do
  kubectl -n $NS create secret generic hnb-pg-$p --from-literal=password="${!p}" \
    --from-literal=dsn="postgres://hnb_$p:${!p}@postgresql:5432/hnb_$p?sslmode=disable"
  kubectl -n $NS create secret generic hnb-$p-nats --from-literal=user="$p" --from-literal=password="$NP"
done
# 平台级密钥
kubectl -n $NS create secret generic hnb-master-key --from-literal=key="$(openssl rand -hex 32)"
kubectl -n $NS create secret generic hnb-admin --from-literal=password="$(openssl rand -hex 24)"
kubectl -n $NS create secret generic hnb-stale-challenge --from-literal=challenge.key="$(openssl rand -base64 48)"
# Provider 服务令牌：键名与 operation-worker.serviceTokenFiles 一一对应（kubernetes）。
# 令牌由 platform-api 的服务身份流程签发；冷启动阶段允许先放空值占位再轮换。
kubectl -n $NS create secret generic hnb-provider-tokens --from-literal=kubernetes="$TOKEN"
# Portal 对外 TLS（域名证书）
kubectl -n $NS create secret tls hnb-tls --cert=server.crt --key=server.key
```

## 4. 安装输入

- `hnb.yaml`：DeploymentConfig（mode=existing、profile=Minimal、namespace=hnb-system、
  三平面 database/role/Secret 引用与上节一致、`storage.className` 指向集群默认 SC）。
- `bindings.yaml`：只含 Secret 引用与组件参数，**禁止** `db.dsn` 等内联凭据；
  `apiserver.sandbox` / `operation-worker.sandbox` 的 digest 从 lock 的 `sandbox` 镜像复制。
- **同一集群目标**：`kubernetes-provider.executorRBAC.enabled=true`，在 `allowedNamespaces`
  的每个命名空间渲染窄权限 `hnb-kubernetes-provider-executor` Role/RoleBinding（Deployment/
  ReplicaSet、Service/Pod/PVC/Secret/ConfigMap、Gateway HTTPRoute）。独立目标集群不要开启，
  应在目标侧 onboarding 中应用等价 RBAC（参考 `deploy/lab/kubernetes-provider-target-rbac.yaml`）。
- **SecretReference 必须显式**：应用 deploy 不再隐式引用 `demo-secret`。仅当目标命名空间已存在
  该 Secret 时传 `secretName`；无 Secret 的工作负载不要传，避免 Pod 因 FailedMount 挂死。

## 5. 渲染 → 安装 → 迁移

```bash
hnbctl render -f hnb.yaml --chart-package hnb-0.1.0.tgz --bindings bindings.yaml \
  --lock release.lock.yaml --output rendered.yaml   # 离线校验：锁、包、绑定一致
hnbctl install -f hnb.yaml --lock release.lock.yaml --runs-dir runs \
  --execute --chart-package hnb-0.1.0.tgz --bindings bindings.yaml \
  --migrations-dir migrations --yes --format json
```

- 迁移按 platform → market → artifact 顺序在各自平面库执行，DSN 从平面 Secret 读取、不打印。
- **DSN 主机解析**：迁移阶段在操作机上直连数据库，DSN 主机名（`postgresql`）必须在操作机
  可解析并可连通。existing 集群路线的标准做法：
  `kubectl -n hnb-system port-forward svc/postgresql 15432:5432 &` 并在操作机
  `/etc/hosts` 加 `127.0.0.1 postgresql`（注意端口转发只绑定到 `[::1]` 时，IPv4 连接会落到
  本机其它监听进程，curl/psql 需显式使用 `[::1]` 或更换端口）。
- 幂等：失败后用同一 `--runs-dir` 重新执行即可续跑；ledger 保证迁移不重复应用。

## 5.1 主机集群 Agent onboarding（Minimal 必做）

cluster-agent 无租户/集群绑定的隧道凭据会拒绝启动。安装后为主机集群完成一次 onboarding：

```bash
# 1) 登记主机集群 target（沿用迁移 096 的种子模式；一次性）
psql ... -c "INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type,
  connection_type, status, lifecycle_state, labels, is_active, created_at, updated_at)
  VALUES ('00000000-0000-4000-8000-00000000000a','default','host-cluster','host-cluster',
  'kubernetes','agent','unknown','ACTIVE','{\"hostCluster\":\"true\"}',true,now(),now())
  ON CONFLICT (tenant_id, name) DO NOTHING;"
# 2) 经平台 API 铸币（admin 登录 → agent-onboarding）
TOKEN=$(curl -s -X POST "$API/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" | jq -r .data.access_token)
curl -s -X POST "$API/api/v1/resources/clusters/00000000-0000-4000-8000-00000000000a/agent-onboarding" \
  -H "Authorization: Bearer $TOKEN" -d '{}' | jq -r .token > agent-token
# 3) 令牌进 Secret，bindings 引用（onboarding.existingTokenSecret/tokenKey/tenantId/clusterId/tunnelUrl）
kubectl -n $NS create secret generic hnb-agent-token --from-file=agent-token=agent-token
# 4) 重新 render + apply，重启 cluster-agent
```

**注意**：静态 onboarding 令牌 TTL 上限 24h，过期后隧道断开，需重新铸币并重启 agent。
生产环境应改用 enrollment/renewable session（`AGENT_ENROLLMENT_SECRET_FILE` 路径，见
`deploy/lab/target-agent.yaml`）；把主机集群 agent 纳入 enrollment 是 S4 5.10 前的待办。

## 6. 验收

```bash
kubectl -n hnb-system get pods            # 19 个工作负载全部 Ready
kubectl -n hnb-system logs deploy/apiserver | grep -i schema   # 平台 schema 已验证
hnbctl status --runs-dir runs --run-id <id>
```

## 7. 回滚 / 卸载

```bash
hnbctl uninstall --installation <id> --plan     # 默认保留数据（PVC/Secret/命名空间）
```

删除数据必须显式 `--delete-data --yes`（交互确认），且遵循平台删除契约。

## 8. 已知边界

- lock 状态为 `candidate`：身份已固定，但独立实施者验收（5.9）完成前不构成 certified。
- 迁移幂等已验证；续跑中的部分失败以 run 状态为准，不允许手工改库后重跑。
