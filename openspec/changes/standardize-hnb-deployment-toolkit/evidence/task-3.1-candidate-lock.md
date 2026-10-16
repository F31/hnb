# 3.1 候选发布锁生成与实际物料身份

- task: 3.1
- requirement: DBND-001
+- date: 2026-10-11
+- status: done（身份解析与生成门禁）；首个完整平台发布锁仍属 5.7
+
+生成锁的 `status: candidate`，只表示物料身份可解析。它不认证档位、路线、离线闭包、
+完整平台安装或依赖闭包完整性；这些仍由 3.2/3.8、5.3–5.7 分别完成。
+
+## 实现
+
+新增 `cmd/hnbctl/internal/toolkit/registry_resolver.go`、`lock_generate.go`、
+`cmd/hnbctl/internal/cli/lock.go`，并扩展 `lock.go`。
+
+发布输入 `release-input/v1` 只声明来源：releaseId/releaseVersion、CLI 文件、已打包 Chart 文件、
+镜像引用、owner 迁移文件、候选兼容声明和允许的重定向来源。CLI 不接受调用方提供任何摘要；
+所有 digest 都由工具从真实字节计算。
+
+- OCI 引用必须是显式 registry/repository 加 tag 或 digest；拒绝 `latest`/大小写变体、
+  路径穿越、无效 repository 段和占位 digest。
+- 匿名 Bearer 只用 challenge 中的 service/scope；校验 scope 必须是本次 repository pull，
+  realm 必须是无凭据 HTTPS；token 只用于当前解析。
+- 解析顺序：index/list → 唯一 `linux/amd64` 目标 manifest → config blob 声明平台 → 校验全部
+  layer descriptor。index 身份与目标架构身份分别记录为 digest/manifestDigest。
+- 内容校验：响应字节 SHA-256 必须等于固定 descriptor；manifest 还比对 `Docker-Content-Digest`。
+- 重定向策略：只允许 HTTPS/绝对/无凭据 URL、≤4 跳、目标主机必须是 registry 同源或
+  发布输入显式声明的来源；**跨主机时不发送 registry token**；内容可信度来自摘要而非重定向主机。
+  实际用到的来源写入锁的 `sourceRedirectOrigins`，不会记录未使用的允许列表。
+- CLI 制品必须是非空常规文件的真实 `hnbctl` Go 二进制（buildinfo 校验模块路径、
+  GOOS=linux、GOARCH=amd64），不接受符号链接，哈希前后各校验一次防并发替换。
+- Chart 必须是打包的 gzip tar，从根 `Chart.yaml` 读取真实 name/version；拒绝 latest、无版本、
+  重复根元数据、超预算归档。
+- 迁移必须是唯一的前向 `.sql`，owner 限定 platform/market/artifact；rollback 文件被拒绝。
+- 输出独占新建 0600；已存在文件在哈希和网络请求之前就被拒绝。
+
+## 自动测试
+
+`cmd/hnbctl` 下 `go test ./... -count=1`、`go vet ./...`、`go test -race ./... -count=1` 全部退出 0。
+
+- `TestRegistryResolvesActualIndexManifestAndConfigBytes`：本地 TLS registry fixture 解析 index→目标 manifest→config。
+- `TestRegistryRejectsUnverifiedIdentityOrArchitecture`：错误 config 平台、错误 digest 头及非法引用拒绝。
+- `TestUndeclaredRedirectIsRefused`：未声明来源被拒；token 不发送到 CDN；声明后可校验内容；
+  非法来源声明拒绝。
+- `TestSourceIdentityAndChartMetadata`：真实 Chart.yaml 身份读取、符号链接拒绝、非打包 Chart 拒绝。
+- `TestCandidateInputRejectsMissingAndInjectedDigests`：注入 digest、缺失物料、缺 CLI/平台身份、
+  伪造 certified 状态、零迁移校验和均被拒。
+- `TestLockGenerateUsageAndSideEffectFreeFailures` / `TestLockGenerateRefusesExistingOutputBeforeResolution`。
+
+## 真机/真实来源验证
+
+真实 registry（匿名 HTTPS，凭据作用域为 `google_containers/pause:pull`）解析
+`registry.aliyuncs.com/google_containers/pause:3.10`：
+
+```bash
+HNB_REGISTRY_IDENTITY_E2E_REF='registry.aliyuncs.com/google_containers/pause:3.10' \
+HNB_REGISTRY_IDENTITY_E2E_REDIRECT_ORIGINS='http://aliregistry.oss-cn-hangzhou.aliyuncs.com' \
+go test -race ./internal/toolkit -run '^TestRegistryRealIdentity$' -count=1 -v -timeout=3m
+```
+
+退出 0；index `sha256:0ca1162b75bf9fc55c4cac99a1ff06f7095c881d5c07acfa07c853e72225c36f`
+与 curl 独立观测的 `docker-content-digest` 一致；目标 manifest
+`sha256:7c38f24774e3cbd906d2d33c38354ccf787635581c122965132c9bd309754d4a`；config 平台
+`linux/amd64`。观察到的内容来源为 `http://aliregistry.oss-cn-hangzhou.aliyuncs.com`
+（阿里云 OSS 对 blob 的 307 明文重定向），由发布输入显式声明并写入锁。
+
+首次尝试未声明来源时按策略失败，保留为设计证据：工具不静默降级到 HTTP。
+
+端到端 `lock generate`（真实 CLI 二进制、真实 `helm package` Chart、真实迁移 SQL）：
+
+```bash
+go build -o /tmp/hnb-lock-e2e/hnbctl .
+helm package deploy/charts/hnb -d /tmp/hnb-lock-e2e   # hnb-0.1.0.tgz
+cp database/postgresql/migrations/101_secret_reference_admission_unique.sql /tmp/hnb-lock-e2e/
+hnbctl lock generate -f release-input.yaml --output release.lock.yaml   # exit 0
+```
+
+生成锁记录：CLI `sha256:6d139bad...efc44`；Chart `sha256:e8b94edd...dd82`（hnb/0.1.0）；
+迁移 `8d437e52...8360`；pause 镜像 digest/manifestDigest 如上；重定向来源已记录。
+独立 `sha256sum` 与锁内记录逐项一致。
+
+## 错误锁门禁（真实执行）

+- 输出已存在 → `INVALID_CONFIG: candidate output already exists`（退出 2）。
+- `pause:latest` → `candidate resolution failed: latest or invalid OCI tag is forbidden`（退出 2）。
+- 重复 YAML 键 → `release input could not be strictly decoded`（退出 2）。
+
+## 未覆盖项
+
+- 仅实现 linux/amd64 解析；多架构 index 选择、registry token 认证、带认证的私有 registry 未支持。
+- 未验证 layer blob 全量下载（闭包属 3.2/4.2）；未构建 HNB 平台镜像，因此锁内只有真实可解析的
+  pause 镜像，不能作为可安装平台发布包。
+- `nodeArtifacts`（OS 包/二进制）仍为空，3.2 未完成；CLI 二进制按字节固定但未做可复现构建证明。
+- 迁移只固定内容校验和，不执行迁移或校验 schema 兼容（3.7）。
+- Helm chart 打包确定性、镜像签名/SBOM/漏洞门禁未纳入本任务。
