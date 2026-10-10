# 功能成熟度与验收证据

`registry.yaml` 是验收台账；源码存在、测试通过、容器启动、真实业务验收分别记录。
历史会话结论不作为发布资格。Blocked 表示目标尚未达成，不得改成 cancelled 隐藏缺环。

## Kubernetes 版本目录

唯一配置键为 `platform_settings.resource.cluster.kubernetesVersions`。
默认目录为空；仅通过真实创建验收且留存报告后才加入精确 patch 版本。
每项必须包含 `code`、`integrationTestStatus: passed`、`integrationReport`。
`integrationReport` 应指向包含 registry 中 report_fields 的不可变验收报告。
离线就绪另外要求 `bundleDigest: sha256:<64 hex>` 和 `offlineReport`。
布尔值只作为声明，不能取代报告/Bundle 校验；目录编辑应仅授权发布管理员。
当前字段引用门禁不是密码学验证报告真实性，CI 签名校验与证据存储仍待交付。

## 集群创建路线

当前可继续验收的路线为 **SSH + kubeadm 单控制面**，不得标为 CAPI 已实现。
管理集群/CAPI 是独立生命周期 owner；在真正的 CRD/controller/BOM 交付前，
禁止将其挂到 SSH 装机后作为“接管”步骤。
SSH success 必须经过 API readyz、精确节点数量、所有节点 Ready、Flannel 与
CoreDNS rollout 检查，才允许保存 kubeconfig 与返回 provisioned=true。
当前仅此 CNI 路线可以验收；其它 CNI、HA、完全离线、证书轮换仍是未验收能力。

验收须用独立目标，不得修改现有业务集群或复用部分失败的节点当作通过证据。
需记录 OS/架构、Kubernetes、kubeadm、containerd、CNI 精确版本和制品摘要。
网络代理由节点管理员配置，脚本不全局清空代理、不自动关闭宿主机 swap。
已安装 kubeadm 必须与目录中的精确 patch 版本一致，不匹配直接失败。
真实验收成功前，生产目录不启用任何版本。
