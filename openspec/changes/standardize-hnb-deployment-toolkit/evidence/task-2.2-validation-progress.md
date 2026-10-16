# 2.2 配置与发布锁校验进展

- task: 2.2
- requirement: DPLY-001, DBND-002
- date: 2026-10-11
- status: partial（tasks.md 保持未勾选）

此文件保留上一轮 partial 状态；后续完成证据见 `task-2.2-validation.md`。

本轮补齐：

- `validate` 默认按配置文件所在目录解析 `spec.releaseLock` 相对路径；显式 `--lock` 按调用路径读取。
- 默认锁缺失/严格解码失败/摘要格式错误时拒绝；请求档位及 provision Kubernetes 版本必须包含于锁的兼容声明。
- 结构与锁检查后执行 `EvidenceGate`；当前完整平台 Minimal 无 E2E，返回 blocked（退出 3）。
- 未知档位、空 evidence matrix 不再静默放行。
- `TimeoutConfig.UnmarshalYAML` 显式拒绝未知嵌套字段，关闭自定义 YAML decoder 绕过 KnownFields 的缺口。
- 拒绝 validate 的额外位置参数及未实现的 `--output`，不静默忽略。

测试：`TestValidateLoadsDefaultLockAndBlocksUncertifiedDelivery`、
`TestRejectsUnknownTimeoutFields`、`TestEvidenceGateFailsClosed`。
在 `cmd/hnbctl` 执行 `go test ./... -count=1`、`go vet ./...`、
`go test -race ./... -count=1` 均退出 0。
测试内重复字符 digest 仅为解析 fixture，不作为真实物料身份。

待完成：配置规范化、模式/策略完整互斥、必填网络/凭据/存储/TLS 字段完善、
重复角色/节点地址及主机网段冲突、实测 OS/arch/内核/runtime 配方与版本组合校验。
发布锁解析和摘要格式检查不证明制品实际存在或依赖闭包完整；实际物料验证属于 3.1/3.2。
