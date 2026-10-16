# Startup 小磁盘规划复测

- run_id: ubuntu-small-disk-planning-20261013
- host: 106.75.34.133
- commit: c23f6e4128e92d7a9457807dce5b10e0b0704e73；工作区含未提交变更，实际 CLI SHA-256 见 result JSON
- command: 真实远端 `hnbctl capacity --manifest <fixture-rendered.yaml> --nodes <actual-nodes.yaml> --scope startup --cache-source external --backup-source external`
- environment: Ubuntu 24.04，2 vCPU，3913Mi，根文件系统实际 18.33Gi，容量输入向下取整 18Gi
- started_at / finished_at: 同名前缀 `-result.json` 中保留 UTC 时间
- exit_code: 0（外部承载假设下规划通过）；3（本地缓存/备份对照正确拒绝）
- scope: 云主机实际执行 CLI 与规划复测，不是 Core Pod/业务部署

## 改动及断言

新增 `startup-small-disk.yaml`，仅覆盖全新测试安装的 PG 2Gi、NATS 1Gi PVC。
显式节点系统盘预留 6Gi。组件与 CPU/内存 requests 保持为 10 个工作负载、
1075m/1792Mi；数据持久化与独立 owner/Secret 绑定保持原契约。

实际节点资源重新观测，上传 CLI 后核对远端二进制 SHA-256，执行容量命令：

1. 外部缓存/备份假设：required = 1683m / 2996Mi / **10Gi**，通过，
   `installAuthorized=false`。
2. 同一实际节点、本地缓存/备份：required disk = **35Gi**，正确拒绝。
3. 本机集成回归另外验证：声明 10Gi 节点 + 显式 6Gi reserve 可通过；保留
   默认 10Gi reserve 时 required disk = 14Gi，正确拒绝 10Gi 节点。

公式为 `ceil(3Gi PVC × 1.1) + 6Gi reserve = 10Gi`。
当前 18Gi 节点即便保留默认系统 reserve、小 PVC、外部缓存/备份假设，14Gi
预算亦可通过；不必为本机强制采用更激进的 6Gi reserve。

## 限制与证据身份

`cache-source external` / `backup-source external` 只是预算放置声明，没有自动
配置外部存储。6Gi 系统预算须容纳 OS、运行时、本地必需镜像/解压层、日志和
临时数据；尚未完成完整镜像集占用实测。没有创建 PVC 或缩小现有磁盘。
fixtures 使用合成 digest 与测试 Secret/DSN，不能作为真实发布包部署。
本次消除了小数据规划的磁盘阻塞，未补齐真实制品、身份/TLS 与 Startup
安装验收，不能宣布整个平台部署成功。10Gi 与云厂商十进制 10GB 不等价。

同名前缀的 `-fixture-rendered.yaml`、`-actual-nodes.yaml`、`-output.txt`、
`-result.json` 留存完整输入与 stdout/stderr，`-artifacts.sha256` 留存摘要。
原默认 Startup 27Gi 的历史测试报告保留，不覆写成此次小盘结果。

本机验证：Go race 测试/vet/build 通过；Chart 与容量集成 **27 tests OK**。
