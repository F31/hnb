# 3.3 Chart 全量 digest 锁定与 Portal 交付资源

- task: 3.3
- requirements: PINST-002, DBND-001
- date: 2026-10-11
- status: done

## 实现

### 单一校验路径

新增 `deploy/charts/hnb/templates/_helpers.tpl`，定义 `hnb.image` 与 `hnb.imagePullPolicy`。
`hnb.image` 对每个组件镜像强制：必须有 `repository`；`digest` 必须匹配
`^sha256:[0-9a-f]{64}$`；拒绝全零占位摘要；按 `global.imageRegistry` 前缀统一重定向。
渲染期直接 `fail`，不会产出半成品清单。

20 个既有组件 chart 原本使用
`{{ .Values.image.repository }}:{{ .Values.image.tag | default $.Values.global.imageTag }}`，
统一改为 `{{ include "hnb.image" . }}`；三个已 digest 化的候选 chart 也并入同一 helper，
消除三份重复校验逻辑。

### 为什么 helper 要复制到每个子 chart

子 chart 无法引用父 chart 的模板。直接对单组件执行 `helm template` / `helm lint`
（组件测试与运维排查单组件时都会这样做）会报 `no template "hnb.image"`——已实测确认。
Helm 对同名 `define` 取最内层作用域，父 chart 渲染时子 chart 自带的副本优先，两条路径行为一致。
因此每个子 chart 的 `templates/_image.tpl` 保留一份副本，并在代表副本中写明必须同步修改。

### 移除 tag 通道

- `global.imageTag` 从 `values.yaml` 删除，替换为说明其为何不存在的注释。
- 删除 calico-provider / cilium-provider / kube-ovn-provider / network-provider 中
  未被任何模板使用的 `imageTag: ""`。
- 未被引用的 `templates/_component.tpl` 不是模板片段（缺少 `define`），Helm 会尝试渲染；
  它仍带 tag 路径。已转为命名模板 `hnb.component` 并改用共享 helper，
  避免留下可被误用的 tag 通道。（未删除，因为删除超出版本锁定范围。）

### Schema

23 个 chart 全部具备 `values.schema.json`，`image.digest` 校验正则与 helper 一致。
默认 `digest: ""` 放行（chart 自身可 lint），实际渲染时由 helper fail-closed 拒绝。

### Portal 交付资源

Portal 具备 Deployment + Service；Ingress 可选，启用时强制 host 与 TLS Secret，
并按需设置 ingressClassName。Nginx 配置通过 ConfigMap 挂载，指向镜像实际的
`/usr/share/nginx/html/shell` 与 `/modules/` 布局，`/api/`、`/tunnel` 同源代理到内部 API。

## 过程中发现并修复的真实缺陷

- **calico/cilium/kube-ovn/network-provider 的 `values.yaml` 存在空的顶层 `global:` 键**
  （null 值），遮蔽了父 chart 的 `global`，导致 `skipping globals because destination
  global is not a table`，`global.imageRegistry` 与 `imagePullPolicy` 对这四个组件失效。
  已删除。这是本任务原本要解决的核心能力（统一重定向）在这四个组件上从未真正生效。
- 补 banner 注释时，注释正文里的 `charts/*/` 含独立 `*/`，提前结束 Helm 模板注释，
  导致 `parse error ... comment ends before closing delimiter`。已改写措辞。

## 验证

```bash
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY no_proxy='*'
helm lint deploy/charts/hnb
helm template hnb deploy/charts/hnb -f /tmp/hnb-all-values.yaml   # 23 个组件全量渲染
python3 deploy/charts/hnb/test_candidate_components.py
```

- `helm lint`：1 chart linted, 0 failed。
- 全量渲染：23 个组件镜像引用，**23 个为 `repository@sha256:...`，0 个残留 tag**。
- `python3 deploy/charts/hnb/test_candidate_components.py`：8/8 通过（Helm v4.2.4+g3900f43）。

新增测试：

| 测试 | 覆盖 |
|---|---|
| `test_every_component_image_is_digest_pinned` | 全量渲染后逐个断言 `@sha256:` 且无 `latest` |
| `test_missing_placeholder_and_bad_digest_are_rejected` | 缺 digest / 全零占位 / `latest` 三类均拒绝 |
| `test_global_registry_retargets_all_images` | `global.imageRegistry` 统一重定向所有镜像 |
| `test_portal_delivery_resources` | Portal 同时产出 Deployment + Service + Ingress，host/TLS/backend 正确 |

其余 4 个为此前候选组件测试（plugin-worker、lifecycle provider 的 Secret/TLS/profile 接线）。

## 门禁（实际执行，全部退出 0）

```bash
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
go vet ./cmd/runtime-target-lifecycle-provider/...
go test -race ./cmd/runtime-target-lifecycle-provider/... -count=1
openspec validate --all --strict          # 29 passed / 0 failed
node scripts/validate-openspec.mjs        # 31 specs / 242 requirements / 377 scenarios
```

## 范围限制

- 验证的是渲染与 lint，未在集群中安装本 Chart，也未启动这些组件容器。
- 测试中的 digest、Secret 名、`192.168.124.12` 等均为渲染 fixture，
  **不是真实物料身份**；真实身份须经 `hnbctl lock generate` 解析（见 task 3.1）。
- Portal 镜像本身尚未构建，`image.digest` 只能填真实发布摘要后才可安装。
- `global.imageRegistry` 默认 `harbor.hnb.local/hnb` 时与默认 repository `hnb/<name>`
  会产生 `hnb/hnb/<name>` 路径；是否需要调整仓库前缀属于 3.6/5.7 的镜像命名决策，
  本任务不做推测性改动。
