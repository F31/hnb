# 引导期镜像收敛到 Harbor + 去除 python3 节点依赖

- change: standardize-hnb-deployment-toolkit
- date: 2026-10-11
- 影响任务: 3.2（配方模型同步修正）、4.1/4.3（provider 行为变更）
- status: 代码、自动测试与真机验收均完成（Ubuntu 24.04 单控制面）

本文件记录一次由用户发起的架构一致性修正，以及由此暴露并修复的四个真实缺陷。
修正前引导路径存在两处会直接阻断 7.2/7.3 断网安装的问题，均由真实验收条件触发
而非代码评审发现。

## 一、缺陷：引导期镜像绕过平台注册表

修正前仓库同时使用三个镜像来源，与「应用市场/镜像仓库统一用 Harbor」的规划冲突：

| 用途 | 修正前 | 修正后 |
|---|---|---|
| 平台自身制品 | Harbor（`pkg/appstore/storage/oci.go`、`HARBOR_URL`、robot 账号） | 不变 |
| kubeadm 控制面镜像 | `registry.aliyuncs.com/google_containers` | `imageRepository` 显式指定 |
| Cilium 数据面/operator | **硬编码 `quay.io/cilium/*`** | `cniImageRepository` 显式指定 |
| Cilium Envoy sidecar | **chart 默认 `quay.io/cilium/cilium-envoy`** | `envoy.image.override` 显式指定 |
| Cilium chart 源 | **硬编码 `https://helm.cilium.io`** | `helmChartRepository`，OCI 引用 |

成因：Rocky 验收时 `registry.k8s.io` 出网不可达，控制面镜像切到阿里云站，
而当时只有 quay 可达，于是 Cilium 留在 quay 并写死在脚本里。这是让测试通过留下的痕迹，
不是架构决策；它使断网安装必然失败，并把供应链信任边界分裂成三处。

### 代码修改

- `provision/types.go`：`ProvisionSpec` 新增 `CNIImageRepository`、`HelmChartRepository`。
- `provision/bootstrap.go`：`NetworkScript` 不含任何上游地址；chart 改为
  `oci://<project>/cilium` 显式引用（Harbor 以 OCI artifact 存 chart，不提供 index.yaml）。
- `provision/orchestrator.go`：`validateSSHSpec` 对 Cilium 路线**强制**要求显式镜像库与
  chart 源，缺失即拒绝；flannel 不受影响。
- `internal/provider/kubernetes.go`：参数契约新增 `cniImageRepository`、`helmChartRepository`。
- `internal/toolkit/node_recipe.go`（hnbctl）：配方模型改为平台仓库来源；
  要求显式 CNI 仓库前缀，拒绝裸 registry host。

## 二、缺陷：节点侧未声明的 python3 依赖

`RuntimeConfigScript` 用 `set -eu` + `command -v python3` 硬性要求 python3，
但 `PrereqScript` 在两种发行版上都**没有安装它**。

- Ubuntu：实测节点存在 `python3-minimal 3.12.3-0ubuntu1`，属镜像预装，故此前验收侥幸通过。
- Rocky：最小化镜像通常不含 python3，该依赖未经验证，两份验收报告均未记录其来源。

### 修改：完全去除

`RuntimeConfigScript` 改为仅用 POSIX awk + 核心 shell 在既有 `imports` 数组中登记配置片段，
不覆盖管理员 `config.toml`；无法识别的 TOML 结构**失败关闭**。
`HNB_CONTAINERD_DIR` 仅供 fixture 测试使用，真实引导不会设置。

## 三、真机验收暴露的另外两个缺陷

### 缺陷 A：kubeadm 镜像 tag 靠猜必然失败

3.2 配方最初硬编码 `coredns:v1.13.1`、`pause:3.10`、`etcd:3.6.5-0`。
真机 `kubeadm init` 立即以 ImagePull 失败告终。节点上实测 `kubeadm config images list
--kubernetes-version v1.36.5` 得到权威清单：

```
kube-apiserver:v1.36.5  kube-controller-manager:v1.36.5  kube-scheduler:v1.36.5
kube-proxy:v1.36.5      coredns:v1.14.2                 pause:3.10.2   etcd:3.6.8-0
```

我猜错 3 个 tag，且漏掉 `kube-proxy`。配方模型改为按 Kubernetes 版本查表
（`kubeadmImageTags`），未登记的版本直接拒绝，不再猜测。

### 缺陷 B：Cilium Envoy sidecar 仍在拉 quay.io

首轮「全部来自 Harbor」的验收中，`cilium-envoy` pod 实际拉取
`quay.io/cilium/cilium-envoy:v1.37.5-...@sha256:75b8094c...`。

Cilium chart 自带该 sidecar，其 tag 由 chart 固定而非 Cilium 版本推导。
我最初据此判断「Envoy 已内嵌、无独立镜像」并从配方中删除 envoy——**这个判断是错的**，
真实部署中它确实被使用。已恢复，并新增 `envoy.image.override` 显式指向平台仓库。
该 tag 记录为 `ciliumChartEnvoyTag` 常量并注明升级 chart 时必须重新核实。

## 四、真机验收证据

环境：Harbor `http://127.0.0.1`（compose 全组件 healthy），节点 `106.75.26.57`
Ubuntu 24.04 / kernel 6.8.0-31-generic / amd64 / cgroup v2。
节点与开发机不在同一网段，用 SSH 反向隧道把 Harbor 暴露到节点 `127.0.0.1:18000`，
隧道只替代网络路径，注册表本身仍是 Harbor。

Harbor 项目（管理权仍限 admin，仅开放匿名拉取供节点 bootstrap）：

| 项目 | 内容 |
|---|---|
| `hnb-cilium` | `cilium:v1.20.1`、`cilium-operator:v1.20.1`、`cilium-envoy:v1.37.5-...`、OCI chart `cilium:1.20.1` |
| `hnb-k8s` | kube-apiserver / controller-manager / scheduler / proxy `v1.36.5`、`coredns:v1.14.2`、`pause:3.10.2`、`etcd:3.6.8-0` |

同步后由 Harbor 直接解析的 digest 与上游逐一一致，例如
`cilium:v1.20.1 → sha256:f70030cc...3ac68d`（与 3.1 从 quay.io 解析结果相同），
chart `sha256:2e66f029...08ac2f1`。

### 执行

前置：`helm uninstall cilium`、`kubeadm reset --force`、清理 CNI/etcd/kubelet pki、
重启 containerd+kubelet，确认 `/etc/kubernetes` 只剩配置与 manifests（非全新 OS 安装）。

```bash
HNB_PROVISION_E2E_SPEC="$(cat /tmp/hnb-harbor-e2e-spec.json)" \
HNB_PROVISION_E2E_KEY_FILE=/tmp/hnb_provision_key \
DB_DSN='postgres://hnb:***@localhost:5432/hnb?sslmode=disable' \
HNB_MASTER_KEY='<64-hex>' \
go test -race ./cmd/runtime-target-lifecycle-provider/internal/provider -run '^TestProvisionRealSSH$' -count=1 -v -timeout=25m
```

spec 中 `imageRepository`/`sandboxImage`/`cniImageRepository`/`helmChartRepository`
全部指向 `127.0.0.1:18000/hnb-*`；密钥、DSN、口令不入库不入证据。

结果 **PASS，44.18s**：7 个 SSH 阶段全部成功，
`accepted nodes=1 endpoint=10.9.127.43:6443 kubeconfig_ref=local-aes/tenant/provision-harborconv00@2 encrypted_at_rest=true`。

### 关键断言：零公共上游

`kubectl get pods -A` 中全部容器镜像：

```
127.0.0.1:18000/hnb-cilium/cilium:v1.20.1
127.0.0.1:18000/hnb-cilium/cilium-operator:v1.20.1
127.0.0.1:18000/hnb-cilium/cilium-envoy:v1.37.5-1786810558-766ccfb37260a43e9d228837aa84ce3faf9f64e7
127.0.0.1:18000/hnb-k8s/kube-apiserver:v1.36.5
127.0.0.1:18000/hnb-k8s/kube-controller-manager:v1.36.5
127.0.0.1:18000/hnb-k8s/kube-scheduler:v1.36.5
127.0.0.1:18000/hnb-k8s/coredns:v1.14.2
127.0.0.1:18000/hnb-k8s/etcd:3.6.8-0
```

无 `quay.io`、无 `registry.k8s.io`、无 `registry.aliyuncs.com`、无 `helm.cilium.io`。

### 数据面

`cilium-2rdvp`、`cilium-envoy-h7x2r`、`cilium-operator`、`coredns`（双副本）、
`etcd`、`kube-apiserver`、`kube-controller-manager`、`kube-scheduler` 均 1/1 Running。
`cilium status --brief` 返回 **OK**；`rollout status` 对 cilium DS、cilium-operator
Deployment、coredns Deployment 全部成功；节点 `cp Ready`，版本 v1.36.5。

### 保留的失败记录

- 首次尝试 tenantID 用占位 UUID → `secret_references` 外键失败，改用真实 `default` 后通过。
- spec 缺 `user` 字段 → 默认以 root 登录导致 `ssh: unable to authenticate`，补 `user: ubuntu`。
- kubeadm ImagePull 失败（tag 猜错，见缺陷 A）。
- CNI 阶段 `helm --repo` 报 `not a valid chart repository` → Harbor 不提供 index.yaml，
  改为 OCI 引用 + `--plain-http`。
- 首轮 envoy 仍来自 quay.io（见缺陷 B），修正后重跑才是全 Harbor 结果。

## 五、门禁（实际执行，全部退出 0）

```bash
go vet ./cmd/runtime-target-lifecycle-provider/...
go test -race ./cmd/runtime-target-lifecycle-provider/... -count=1   # provision 1.283s / provider 1.155s
cd cmd/hnbctl && go vet ./... && go test -race ./... -count=1        # cli 1.062s / toolkit 1.444s
```

新增/更新的测试包括：Cilium 拒绝隐式上游、`RuntimeConfigScript` 无解释器依赖、
containerd import 合并的 7 组 fixture 与幂等/失败关闭（执行渲染后的真实脚本文本，
stub 掉 containerd/systemctl）。

## 六、范围与限制

- 仅 Ubuntu 24.04 单控制面完成本轮真机验收。Rocky 10.1 的 python3 依赖问题已从代码上消除，
  但该 OS 的 Harbor 路线尚未重跑。
- 反向隧道替代了网络路径，不代表真实跨网段/断网环境；真正的禁公网验收仍属 7.2/7.3。
- 节点软件包（kubelet/kubeadm/containerd/python3 等）仍来自 OS 与 k8s 频道，
  未被发布锁固定——3.2 的闭包缺口结论不变。
- 本次未验证 Harbor proxy-cache；镜像是直接同步进 Harbor 的。
- 平台自身 HNB 组件镜像仍未构建，不构成可安装平台发布包（仍属 5.7）。
