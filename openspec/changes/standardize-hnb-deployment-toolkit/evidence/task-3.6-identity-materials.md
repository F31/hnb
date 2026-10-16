# 3.6 实例身份、KMS 材料与管理员初始化生成

- task: 3.6
- requirements: PINST-003, DPLY-007
- date: 2026-10-12
- status: done

## 实现

`cmd/hnbctl/internal/toolkit/identity.go` + `identity_util.go`，CLI `hnbctl identity generate`。

一次生成并严格校验：

- **实例身份**：EC P-256 私钥（0600）+ 公钥（0644）+ key manifest。
  - manifest 含 issuer、generation、activeKeyId，以及 active key 的
    `NotBefore`（生成前 5 分钟）/`NotAfter`（`--validity-days`，默认 30 天）窗口。
  - 生成即 active，不产生 pending 状态；时间窗口是安装校验的硬前提。
- **KMS 材料**：32 字节随机 AES-256 主密钥，hex 写入 `master.key`（0600），
  写出后用 `kmsMasterKeyFromHex` 反向校验可解码且恰为 32 字节。
- **管理员初始化**：24 字节随机一次性密码，bcrypt 哈希存 `AdminCredential.Hash`，
  明文只写 `admin-bootstrap.password`（0600），`ExpiresAt = now + AdminExpiry(24h)`。
- **服务凭据**：明确区分实例身份与在线服务凭据——离线安装器**不**签发访问令牌
  （`GenerateServiceCredential` 显式报错），因为那需要在线 TokenManager/身份库；
  服务凭据由平台上线后按 audience 颁发，避免把安装器变成执行旁路。

### 权限与入口校验

- `writeRestricted`：私密材料仅允许 0600，公开材料（public key/manifest）允许 0644，
  其余模式一律拒绝；写出后 `os.Stat` 复核权限。
- issuer 必须是 HTTPS + 尾部斜杠；publicBaseURL/entry 必须是 HTTPS；
  audience 非空且无空白；validity 限 24h..1y；输出目录必须绝对路径。
- **重放保护**：`identity-state.json`（0600）记录 issuer/keyId/generation/manifest 哈希；
  对同一 issuer 再次生成直接拒绝，防止恢复/重跑时静默换新密钥。

### 不泄露

- `AdminCredential.Plaintext` 标 `json:"-"`——material JSON、state 文件、evidence
  都不含明文密码（有测试断言）。
- CLI 在 stdout 一次性展示明文给操作员，stderr 提示首次登录后必须轮换。

## 测试（`identity_test.go`）

| 测试 | 覆盖 |
|---|---|
| `TestIdentityMaterialIsInsideValidityWindow` | 生成时 key 必在窗口内 |
| `TestIdentityMaterialRejectsExpiredWindow` | 窗口外拒绝并报 "validity window" |
| `TestIdentityFilesArePermissionRestricted` | 私密 0600 / 公开 0644 |
| `TestIdentityPrivateKeyIsUsable` | PEM 可解析、可签名 |
| `TestIdentityGenerationRejectsReplay` | 同 issuer 二次生成拒绝 |
| `TestAdminCredentialIsStrongAndBounded` | 密码长度、过期边界、state 不含明文 |
| `TestIdentityManifestLoadsWithPlatformLoader` | manifest 可被平台加载器解析 |
| `TestIdentityPlanRejectsInvalidInputs` | 6 类非法输入写出前拒绝 |
| `TestIdentityMaterialJSONNeverContainsPlaintext` | JSON 序列化不含密码 |

## 真实 CLI 执行

```bash
/tmp/hnbctl identity generate --issuer https://issuer.hnb.local/ \
  --audience hnb-apiserver --entry https://portal.hnb.local \
  --dir /tmp/hnb-identity-demo --validity-days 30
```

输出验证：
- 目录 0700；`private.pem`/`master.key`/`admin-bootstrap.password`/`identity-state.json` 均 0600；
  `manifest.json`/`public.pem` 0644。
- 二次执行同一 issuer → `INVALID_CONFIG: ... refusing replay`，退出码 2。

## 门禁

```bash
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
openspec validate --all --strict        # 29 passed / 0 failed
node scripts/validate-openspec.mjs      # 31 specs / 242 requirements / 377 scenarios
```

## 范围限制

- 未验证证书 SAN 与真实客户入口匹配（3.6 校验 HTTPS 形态，不校验 DNS 解析/证书链；
  实际证书由 TLS existingSecret/入口提供）。
- 服务凭据在线签发未实现——按设计不属于离线安装器。
- 生成本身在 `hnbctl`，未接入 apiserver 启动自动生成；安装编排接身份属 4.x。
