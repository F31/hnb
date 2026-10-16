# 2.3 交互与非交互配置草稿

- task: 2.3
- requirement: DPLY-001, DPLY-008
- date: 2026-10-11
- status: done

## 实现

`cmd/hnbctl/internal/cli/run.go` 使用标准 flag 解析器接受两种模式：

```bash
hnbctl init --mode existing --profile Minimal -f hnb.yaml
hnbctl init --mode bootstrap --os ubuntu-24.04 -f ubuntu.yaml
hnbctl init --mode bootstrap --os rocky-10.1 -f rocky.yaml
hnbctl init --interactive
```

交互模式询问模式、档位、bootstrap OS 和输出路径，使用同一模板生成器。
bootstrap Ubuntu 默认用户 `ubuntu`，Rocky 默认用户 `root`；OS 选项仅选择候选模板，
不代替未来预检的实际 OS 观测。模板保留 kubeadm `1.36.5`、Cilium `1.20.1`。
入口、存储、TLS、SSH/kubeconfig 路径须由实施人员填写，release lock 须提供真实文件。
草稿不包含真实凭据、虚构入口或摘要，且未填写草稿不能通过结构校验。

文件使用 O_EXCL 新建、权限 0600；拒绝覆盖已有文件及符号链接。
未知选项、缺少选项值、额外位置参数、未知 OS 和未认证 HA 档位在写文件前拒绝。
非交互缺参直接报错；只有显式 `--interactive` 才读取 stdin。
草稿生成成功不代表 Minimal 或 bootstrap 已通过完整平台认证。

## 自动验证

工作目录 `cmd/hnbctl`：

```bash
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
```

本轮全部退出 0。关键测试：

- `TestInitDrafts`：existing/Ubuntu/Rocky 均严格解码，用户选择正确，0600 权限，未填草稿被拒绝。
- `TestInitInteractiveMatchesNoninteractive`：交互与非交互 Rocky 草稿逐字节一致。
- `TestInitRefusesOverwriteAndSymlink`：已有配置内容保持不变。
- `TestInitInvalidInputDoesNotCreateFile`：所有错误输入无输出文件副作用。
- `TestMatrixShowsBlockedProfiles`：不扩大完整平台认证范围。
