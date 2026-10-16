# 5.2 脱敏诊断 collect 与报告输出身份

- task: 5.2
- requirements: DPLY-007, DBND-005
- date: 2026-10-12
- status: done（归档构造、脱敏与包身份）；真实服务日志采集属 5.3+ 环境接线

## 实现

新增 `internal/toolkit/collect.go` 与 `hnbctl collect`。

### 两级入口，结构优先

- **`AddJSON`（结构化）**：先 marshal，再经 `RedactMap` 按键丢弃
  （`password/passwd/pwd/secret/token/api-key/private-key/authorization/kubeconfig/dsn/url`）。
  结构化条目只应承载摘要、原因码与状态，凭据字段即使被传入也会被整体丢弃。
- **`AddText`（自由文本）**：经 `Redact` 正则脱敏，并按 `MaxLogBytes`
  （默认 64 KiB）截断且标注 `[TRUNCATED]`，防止归档被用作夹带客户数据的通道。

### 归档与身份

- 确定性 gzip tar：条目按路径排序、固定时间戳/权限 0600，**相同输入产生逐字节相同的归档**。
- `BundleIdentity`（`diagnostic-bundle/v1`）输出 `bundleId`、`runId`、`collectedAt`、
  `redacted=true`、`entryPaths` 与覆盖全部脱敏内容的 `contentDigest`。
- 内容摘要基于**脱敏后**字节，反映接收方实际得到的内容。
- 条目名拒绝绝对路径、`..`、换行与重复；输出文件 `O_EXCL` 独占创建、0600，
  不覆盖既有归档。

### 收集内容

`install/run-summary.json`（runId/configDigest/lockDigest/ownership/各阶段状态与原因码）、
`tool/identity.json`（工具版本与采集时间）、`diagnosis/reason-codes.json`
（原因码与操作卡引用），以及 `bundle-identity.json`。

## 验证要求对照

**密码/私钥/kubeconfig/token/DSN/客户数据 fixtures 不出包**：
`TestCollectArchiveContainsNoSecretsOrCustomerData` 构造 10 类敏感 fixture
（含 `Contoso Retail Ltd`、`contoso-prod`、`acme-billing` 等客户数据），
写入结构化条目与日志摘录后解包，断言归档字节中**不含**任何一项，
同时确认脱敏未误伤诊断证据（`configDigest` 等摘要仍保留）。

补充覆盖：
- `TestCollectReportsBundleIdentityDeterministically`：身份确定性 + 归档字节可复现
- `TestCollectContentDigestTracksRedactedContent`：内容变化即摘要变化
- `TestCollectRejectsUnsafeEntryNames`：路径穿越/重复名拒绝
- `TestCollectTruncatesOversizedLogExcerpts`：超限截断并标注
- `TestCollectArchiveFileIsPrivateAndExclusive`：0600 + 拒绝覆盖
- CLI：`TestCollectCLIProducesIdentityAndEvidence`、`TestCollectRefusesToOverwriteArchive`、
  `TestCollectRequiresOutput`、`TestCollectRejectsUnknownRunDir`

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

`collect` 只打包本机已持有的运行状态与调用方提供的失败信息；它**不**主动连接
集群/平台抓取服务日志。真实服务日志、节点观测与目标侧证据的采集接线属 5.3+
环境验收。脱敏为模式与键名驱动，属尽力而为；归档本身标记 `redacted=true`，
接收方仍应按敏感数据处理。
