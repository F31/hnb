# 2.6 预检报告与脱敏输出

- task: 2.6
- requirements: DPLY-002, DPLY-005, DPLY-007
- date: 2026-10-11
- status: done（报告契约与输出层，不代表完整检查集已实现）

## 实现

`cmd/hnbctl/internal/toolkit/report.go`、`report_output.go`，以及 CLI preflight/dispatch：

- 报告 Schema `preflight-report/v1`，包括 runId、configDigest、lockDigest、generatedAt、checkSetComplete 和 checks。
- 生成时间置于所有观察之后；输出统一 UTC。观察时间缺失或晚于报告时间时拒绝输出。
- 校验 checkId/reasonCode/scopes/node 身份、状态和原因码的一致性、必填 expected/actual、失败修复说明。
- scope/node/checkId 的重复身份拒绝；不同节点允许使用同一检查类型。不可将 incomplete marker 宣称 complete。
- `WritePreflightReport` 先校验，再在副本中脱敏，human/JSON 使用相同安全投影，不修改原始证据。
- 描述字段若包含密码/token/KMS key、Bearer/JWT、DSN、带认证或 query 的 URL、PEM（包括不完整私钥）或 kubeconfig 内容标记，整个字段替换为 `[REDACTED]`，不只删除字段名。
- 控制字符过滤；human 的 expected/actual/remediation 使用 quoted strings，防止注入终端行/ANSI 控制。
- 不采集任意远端日志/响应体/凭据内容；不能从一个裸、无标签的任意字符串推断其是否为秘密。完整诊断归档和客户数据清理仍属 5.2。
- CLI 失败消息同样使用安全文本处理并带稳定原因码前缀。正常 JSON stdout 不混入人类摘要/错误消息。
- 非零退出区分 `PREFLIGHT_BLOCKED`（3）与 `OUTPUT_FAILED`（1）；writer/短写错误不能冒充报告成功。
- `--output` 文件维持独占新建/0600/拒绝覆盖；始终输出 JSON，与 JSON stdout 的投影逐字节一致。

## 使用方式

需提供真实已填写配置和可解码的发布锁；下面命令不表示当前发布包或完整平台已认证：

```bash
hnbctl preflight -f hnb.yaml --format human
hnbctl preflight -f hnb.yaml --format json
hnbctl preflight -f hnb.yaml --format json --output report.json
```

默认 stdout 为 human。报告中的 `checkSetComplete=false` 和
`PREFLIGHT_CHECK_SET_INCOMPLETE` 保留 2.5 缺口，当前整体退出 3。
错误消息写 stderr，不污染 JSON stdout。非法 format 在读取配置、发出网络请求及写文件之前拒绝。

## 验证

在 `cmd/hnbctl` 实际执行以下命令，均退出 0：

```bash
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
```

关键测试：

- `TestPreflightHumanAndJSONPreserveContract`：版本、摘要、UTC 时序、expected/actual/remediation、稳定身份与原证据不变。
- `TestPreflightRejectsInvalidContractBeforeOutput`：10 类无效契约在输出字节前拒绝；不同来源节点不误判重复。
- `TestPreflightRedactsEntireSensitiveFieldsInBothFormats`：14 组密码、JSON/ENV token、KMS、签名 URL、DSN、NATS、PEM/kubeconfig/JWT；human 和 JSON 的三个描述字段全部隐藏，原字段不变。
- `TestReportOutputErrorsAndTerminalControls`：writer 失败、短写、终端行/控制字符与 JSON 解码后的安全性。
- `TestPreflightPartialReportCannotAuthorizeInstallation`：offline CLI fixture 无公网请求；JSON stdout 可直接解析且与文件一致，报告有效但不 complete；stdout 写入失败返回 OUTPUT_FAILED。
- `TestPreflightRejectsBadFormatBeforeReadingOrWriting`：非法格式不读取不存在配置、不写报告。

测试 fixture 仅用于输出层，不作为真实发布锁、物料、入口或集群验收。
本轮不访问已删除的 Rocky 主机，也不重复远端节点安装/初始化。

## 最终门禁

`go build -o /tmp/hnbctl .` 通过；`/tmp/hnbctl help preflight` 展示 human/json 与 report.json 参数。
`openspec validate --all --strict`：29 passed / 0 failed；
`node scripts/validate-openspec.mjs`：31 specs / 242 requirements / 377 scenarios；`git diff --check` 通过。
2.6 已勾选，累计 10/48；2.5、3.1/3.2 和完整平台认证继续保持各自原状态。
