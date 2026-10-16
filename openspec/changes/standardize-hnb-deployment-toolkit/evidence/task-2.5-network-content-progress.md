# 2.5 网络、入口 TLS 与内容可达性检查（完成）

- task: 2.5
- requirements: DPLY-002, PINST-003, DBND-002
- date: 2026-10-12
- status: done（tasks.md 已勾选）

## 检查集清单

配方驱动的网络/入口/内容可达性检查，按源路径独立观察，不互相推导：

### 节点 -> API 路径（provision）
`ProbeNodeAPIPaths`：分别从每个声明节点经只读 SSH 观察该节点 -> API TCP（6443），
不从安装机或 SSH 成功推导 worker 路径。bash/timeout 缺失 blocked，不安装工具；
地址/端口严格校验后才生成固定 TCP 检查。TCP pass 不代表 API readyz、认证、TLS、
集群/CNI 或平台可用。

### 入口 TLS（installer -> HTTPS entry）
`ProbeEntry`：正常 CA/SAN/有效期校验，无 skip-verify 开关；HTTP 2xx 才算 pass，
redirect/登录响应不视为完整入口成功。

### 内容元数据 vs 完整下载（installer -> pinned 物料）
`ProbeContent`：HEAD metadata 与 GET 内容独立 verdict；流式读取、无缓存文件、
完整 SHA-256、截断流、下载预算（blob/package 64 MiB、manifest 1 MiB）超限 blocked，
不能把已读部分当下载完成。HEAD 200 永不替代 GET 校验。

### OCI 单 manifest / 多架构 index（installer -> registry）
`ProbeLockedArtifacts`/`artifact_probe`：单 manifest 按锁定 digest 验证原始内容，
再验证 config/layer 的 HEAD、GET、摘要与 descriptor size；index 无目标架构 recipe 时
blocked，不绕过认证/redirect/source mapping。

### existing 集群 API 源路径（本轮新增）
`ProbeExistingKubeconfig`：读取 kubeconfig，校验文件完整性/YAML/current-context、
server 必须为无凭据 https、CA（内嵌 data 或 file）可解析进验证池、
installer -> API server 实际 TLS 握手以声明 CA 验证。拒绝 insecure-skip-tls-verify、
http、凭据 URL、坏 YAML、无 CA；不读凭据、不执行插件、不改文件。

## 差异 fixtures 验收（自动测试）

- `TestSSHReachabilityCannotProveAPIPath`：节点 SSH/系统观察通过、API TCP 不通仍 blocked。
- `TestNetworkObservationsFailClosed`：工具缺失/额外凭据行/非法目的端口拒绝。
- `TestContentMetadataDoesNotProveDownload`：HEAD 200 / GET 503 保留不同 verdict，无响应体泄密。
- `TestContentIdentityCompletenessAndBudget`：真实 fixture 字节摘要、摘要不符、超限、截断流分判。
- `TestProbeNeverFollowsRedirectsOrRequestsCredentialURLs`：redirect destination 未收到请求；敏感引用请求前拒绝。
- `TestEntryTLSChainSANAndValidity`：有效/过期/SAN 不符/未知 CA；后者 blocked。
- `TestOCIManifestMetadataCannotProveLayerDownload`：manifest/config 可读、layer HEAD 200 但 GET 403 时 blocked。
- `TestOCIIndexCannotClaimArchitectureClosure`：index 身份验证通过不代替架构依赖闭包。
- `TestOCIDescriptorSizeMustMatchBytes`：摘要正确但 descriptor size 不同仍 blocked。
- `TestKubeconfigProbe*`（本轮新增）：文件/server 引用、端到端 TLS、错误 CA、不可达 server、坏 YAML/无 current-context。

TLS/OCI/内容 fixture 均为本地测试服务和真实计算的 fixture 摘要，不是正式发布物料或客户环境证据。

## 接入 preflight

`runPreflight`：provision 分支执行节点探针 + API TCP；existing 分支执行
`ProbeExistingKubeconfig`。offline 分支明确 `OFFLINE_CLOSURE_UNVERIFIED`，
不以在线检查结果冒充离线闭包。所有缺口汇入 `PREFLIGHT_CHECK_SET_INCOMPLETE`，
整体 preflight 不授权安装（仍返回 ReasonPreflightBlocked）。

## 真机观察（Ubuntu，读只）

复用 `106.75.26.57`（Ubuntu 24.04，用户 ubuntu + existing sudo，严格 existing host-key），
无节点变更，配置/package before=after 一致：

- `node.api-tcp`：source=hnb-ubuntu-cp -> 10.9.127.43:6443 pass / CHECK_PASSED。
- `node.owner-markers`：kubernetes，blocked / NODE_OWNER_PRESENT，仍拒绝再次 bootstrap。

existing 集群的 kubeconfig API 真机观察留待 5.3 独立已有 Kubernetes 环境 E2E，
届时以真实集群 API 验证本探针端到端。

## 门禁

```bash
cd cmd/hnbctl && go test ./... -count=1   # PASS
go vet ./...                                # PASS
go test -race ./... -count=1               # PASS
```

## 范围说明

TCP pass 只代表源节点到 API 端口可连接，不代表 API readyz、认证、TLS 或平台可用。
installer 内容下载 pass 不代表节点/Provider 的包源和镜像拉取路径可用。
这些更深的源路径在 4.x/5.x 的执行与验收任务中继续验证。
