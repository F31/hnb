# 2.1 hnbctl 骨架

- task: 2.1
- requirement: DPLY-001, DPLY-008, DBND-002
- date: 2026-10-11
- status: done

## 交付物

| 文件 | 内容 |
|---|---|
| `cmd/hnbctl/main.go` | signal-aware 入口，`os.Exit(Execute(...))` |
| `internal/cli/exitcodes.go` | 结构化退出码：0 success / 1 error / 2 usage / 3 blocked / 4 not-verified / 5 connection-failed；稳定 `ExitReason` |
| `internal/cli/commands.go` | 命令注册表：version/help/matrix/init/validate/preflight/prepare/install/status/resume/verify/collect/cluster/node/platform/operation |
| `internal/cli/run.go` | 各命令实现；未交付机器返回明确 non-success，绝不打印假成功 |
| `internal/cli/cli_test.go` | CLI 行为测试 |

## 命令行为

| 命令 | 行为 | 退出码 |
|---|---|---|
| `help` / `version` / `matrix` | 列出命令/版本/证据矩阵 | 0 |
| `init --mode existing\|bootstrap --profile Minimal` | 生成 0600 权限草稿，必填项留空 | 0 / 2（不支持 profile） |
| `validate -f x.yaml [--lock ...]` | 当前结构校验；2.2 支持矩阵和发布锁集成尚待完成 | 0 / 2 |
| `node add` | `TARGET_ACTION_UNSUPPORTED`（服务端无动作契约） | 3 |
| `install/preflight/prepare/status/resume/verify/collect/cluster/platform/operation` | 明确 `not implemented yet` + 后续任务引用 | 非 0 |

## 测试证据（`go test -race ./... && go vet ./...` 全部通过）

| 用例 | 结果 |
|---|---|
| help 列出全部命令 | PASS |
| 未知命令返回 usage | PASS |
| version 退出 0 | PASS |
| matrix 展示 StandardHA blocked + existing 为优先实现路线；完整平台认证仍待 E2E | PASS |
| init existing 生成含必填空字段的模板，无 latest/假入口 | PASS |
| init StandardHA 拒绝并给原因 | PASS |
| validate 拒绝非法配置 | PASS |
| node add 返回 TARGET_ACTION_UNSUPPORTED | PASS |
| install 未实现时不得报告成功 | PASS |
| init 模板因空 publicBaseURL 过不了 validate（不兼容 fixture） | PASS |

## 验证命令

```bash
cd cmd/hnbctl && go test -race ./... -count=1 && go vet ./...
go build -o /tmp/hnbctl .
/tmp/hnbctl node add --target x   # 退出码 3
```

smoke：`/tmp/hnbctl help`、`version`、`matrix`、`node add`（退出 3）均符合预期。

## 2026-10-11 核对更正

原文将 2.2/2.3 合并标记完成，但未覆盖交互初始化、Rocky 模板和默认发布锁读取。
本证据仅证明 2.1；2.3 的补充实现见 `task-2.3-init.md`；2.2 当时保持未完成，后续完成证据见 `task-2.2-validation.md`。
本轮实际执行 `go test ./... -count=1`、`go vet ./...`、`go test -race ./... -count=1` 均退出 0。
