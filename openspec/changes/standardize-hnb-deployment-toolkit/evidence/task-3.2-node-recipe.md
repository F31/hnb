# 3.2 节点依赖配方与闭包检查

- task: 3.2
- requirements: DBND-001, DBND-003
- date: 2026-10-11
- status: done（配方解析与闭包缺口报告）；闭包尚未闭合，不构成可安装发布

## 实现

`cmd/hnbctl/internal/toolkit/node_recipe.go` 按 OS/arch 解析节点依赖，
条目可追溯到已发布 provision 脚本：

- `PrereqScript`（bootstrap.go）：APT 分支安装 `apt-transport-https`、`ca-certificates`、
  `curl`、`gpg`；两分支安装 `kubelet`/`kubeadm`/`kubectl`（固定 1.36.5）与 `containerd`。
- `RuntimeConfigScript`（bootstrap.go）：两分支都要求 `python3`，用于在不覆盖管理员配置的
  前提下合并 containerd import。
- `NetworkScript`（bootstrap.go）：Helm 拉取 `quay.io/cilium/cilium:v1.20.1` 与
  `quay.io/cilium/operator-generic:v1.20.1`。
- `orchestrator.go`：Helm 必须是预置 CLI，否则引导直接失败。
- kubeadm/`imageRepository` 拉取 apiserver、controller-manager、scheduler；
  containerd 拉取 sandbox/pause；CoreDNS 属于验收门等待的系统组件。

配方因此为每个 OS 给出包管理器、软件包（标注来源）、镜像和预置工具清单。
只接受有证据的组合（ubuntu-24.04 / rocky-10.1、amd64、cilium 精确版本、显式 imageRepository）；
拒绝未知 OS、arm64、flannel、`latest`、缺仓库和缺 sandbox 引用。
**不回落 `registry.k8s.io`**：Rocky 验收中该源出网不可用。

`CheckClosure` 把配方与候选锁比较：镜像按 ref 精确匹配（同名不同 ref 不算满足）；
渠道解析的软件包与预置工具**显式标记为未固定**，返回缺口而非静默通过。

CLI `hnbctl recipe show -f hnb.yaml --os ... --image-repository ... --sandbox-image ...`
打印配方、逐条列出闭包缺口，并在存在缺口时返回 `PREFLIGHT_BLOCKED`（退出 3）。

## 自动测试

`cmd/hnbctl` 下 `go test ./... -count=1`、`go vet ./...`、`go test -race ./... -count=1` 退出 0。

- `TestRecipeMatchesProvisionScripts`：两种发行版都含 kubelet/kubeadm/kubectl/containerd/python3；
  APT 专有包只在 Ubuntu 出现；Cilium/operator/envoy/pause 齐备；无 `registry.k8s.io`。
- `TestRecipeRejectsUnsupportedInputs`：7 类未支持输入拒绝。
- `TestRecipeClosureReportsUnpinnedNodeDependencies`：缺锁、仅固定 sandbox、缺镜像、
  不兼容锁、换 ref 等场景；渠道包与预置工具保持未固定表述。

测试暴露并修复了一个真实缺陷：`python3` 最初被误列为 APT 专属，
而 `RuntimeConfigScript` 在两种发行版上都需要它。

## 真实验证

`HNB_NODE_RECIPE_E2E_IMAGES` 解析真实 registry 字节（匿名 HTTPS，token 仅限 pull scope）：

```bash
HNB_NODE_RECIPE_E2E_IMAGES='registry.aliyuncs.com/google_containers/pause:3.10,quay.io/cilium/cilium:v1.20.1,quay.io/cilium/operator-generic:v1.20.1' \
HNB_REGISTRY_IDENTITY_E2E_REDIRECT_ORIGINS='http://aliregistry.oss-cn-hangzhou.aliyuncs.com,https://cdn01.quay.io' \
go test -race ./internal/toolkit -run '^TestRecipeRealNodeImageResolution$' -count=1 -v -timeout=5m
```

退出 0，21.69s，三个镜像均确认为真实 `linux/amd64`：

| 镜像 | index/root | 目标架构 manifest |
|---|---|---|
| pause:3.10 | `sha256:0ca1162b…25c36f` | `sha256:7c38f247…754d4a` |
| cilium:v1.20.1 | `sha256:ae9ea21f…a71d11b` | `sha256:f70030cc…3ac68d` |
| operator-generic:v1.20.1 | `sha256:6c3885fc…d16ecf` | `sha256:8b613c2e…213663` |

quay.io 把 config blob 302 重定向到 `https://cdn01.quay.io` 的预签名 URL；
未声明该来源时按策略失败，声明后通过校验。该失败保留为设计证据。

真实闭包检查（`recipe show`，ubuntu-24.04 + 上述仓库）退出 3，报告 **18 条缺口**：
cilium/operator/envoy、三个控制面镜像、CoreDNS 未在锁内；
kubelet/kubeadm/kubectl（k8s 频道）、containerd 与 APT/python3（OS 频道）无锁内身份；
helm 与 sudo 为预置依赖、未被发布固定。

## 结论与未完成项

## 后续修正：引导期镜像收敛到 Harbor（真机验收已完成）

原表中 kubeadm 镜像 tag 与 Cilium Envoy 结论均被真机证伪并修正：
`coredns:v1.13.1`/`pause:3.10`/`etcd:3.6.5-0` 实际应为 `v1.14.2`/`3.10.2`/`3.6.8-0`
（并补 `kube-proxy`）；`cilium-envoy` 被误判为已内嵌，实际由 Cilium chart 以独立 sidecar
拉取。配方模型已按 kubeadm 版本查表，并恢复 envoy 依赖。
详见 `harbor-convergence-and-python3-removal.md`。

## 后续修正：引导期镜像收敛到 Harbor

`quay.io/cilium/*` 原为 `NetworkScript` 硬编码，已参数化为平台仓库
（`cniImageRepository`/`helmChartRepository`），配方模型同步调整。
详见 `harbor-convergence-and-python3-removal.md`。本节缺口结论不变：
即使镜像来源收敛为 Harbor，节点软件包与预置工具仍未被发布锁固定。

配方解析与缺口报告已完成，但**闭包未闭合**：当前没有任何一个节点的完整依赖集合被发布锁固定。
后续需要：把节点软件包/二进制纳入锁身份（3.1 扩展或 4.2 prepare）、
补齐控制面/网络/DNS 镜像、消除对预置 helm 的隐含依赖，并为 envoy 取得独立证据。
本任务不代表 3.8 容量预算或 5.x 平台验收。
