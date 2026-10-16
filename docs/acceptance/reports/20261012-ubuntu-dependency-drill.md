# Ubuntu 云主机依赖层重启与备份恢复实验

- run_id: ubuntu-dependency-drill-20261012
- date: 2026-10-12
- source: 工作树中的固定 `NodeProbeScript`；实验脚本及原始输出随本报告保留
- environment: 106.75.10.155 / Ubuntu 24.04 / kernel 6.8.0-31 / amd64 / 2 vCPU / 4007932 KiB RAM / 19 GiB root disk
- status: passed（依赖层实验）；完整 5.6、7.2、7.3 保持未完成
- requirements: DPLY-002、PINST-004、PINST-006、DBND-005 的部分证据

## 命令与真实结果

1. 使用 `SSHNodeProbe.Run` 执行仓库固定 `NodeProbeScript`，对比 `/etc/fstab`、`/etc/sysctl.conf`、containerd/kubelet 配置和 dpkg 软件清单的前后摘要。一致；owner=none。原始输出包含脚本与快照摘要。
2. 显式执行 `apt-get update`、`apt-get install postgresql-16 nats-server`。联网从空 OS 安装，版本为 PostgreSQL 16.15、NATS 2.10.7；不是 release-lock 固定发布或离线安装。
3. `sudo python3 hnb-cloud-drill.py seed`：创建三个专属测试 owner/数据库，分别写入一行数据；创建 JetStream 文件流，写入一条确认消息；生成两个 32-byte 随机测试材料文件（0600）。
4. `sudo python3 hnb-cloud-drill.py backup`：PG 使用 `pg_dump -Fc`；JetStream 停止专属实例后复制文件存储再启动；复制材料文件；记录九个备份文件的 SHA-256。
5. `sudo systemctl reboot`：真实节点重启，boot ID 从 `d8b59c40-dfef-45b3-9abb-f1be3cb9dea9` 变为 `d93698a1-7663-45ca-a67d-cfb0f0f6f600`。
6. 重启后 `verify`：三个数据库的数据、JetStream 消息数量/内容、材料摘要与权限均通过断言。
7. `restore`：核验备份摘要；PG 恢复到三个新 `_restored` 数据库；主动删除本实验流，再停止专属 NATS 实例并恢复其文件存储；覆盖测试材料后由备份恢复；最终内容断言全部通过。脚本测量恢复与验证约 0.672 秒。
8. `SET ROLE hnb_drill_platform; SELECT * FROM drill;` 在 market 测试库返回 permission denied，确认跨 owner 表访问被拒绝。

阶段 seed/backup/verify/restore 最终退出码均为 0。首次重连验证因 `/tmp` 脚本被重启清理而失败；将相同脚本重新上传到 `/home/ubuntu` 后继续，未重新 seed 或重建数据，失败输出保留。

## 范围与留存

这是小数据集、停服一致性备份的依赖实验。身份/KMS 文件是随机测试字节，不是 HNB 实例身份、实际 KMS 解密或管理员认证。没有部署 HNB、执行正式 Operation、验证 Agent、禁公网安装或批准的 RTO/RPO。完整 5.6 仍为 partial，7.2/7.3 未通过。

主机留有 `/var/lib/hnb-cloud-drill-20261012`（数据、备份与摘要）、`/home/ubuntu/hnb-cloud-drill.py`、三个原始和三个恢复数据库，以及 `hnb-cloud-drill-nats.service`。PG/NATS 仅监听 loopback；包自带 NATS 默认服务已停止禁用。SSH 已添加现有实验公钥。资源最终约占 460 MiB 内存、2 GiB 磁盘。

- [实验脚本](20261012-ubuntu-dependency-drill.py)
- [保留原始输出](20261012-ubuntu-dependency-drill-output.txt)

本地通过目标主机专用路由绕过默认 eth5 链路后可稳定连接；这支持网络路径干扰判断，但不能仅凭此确定具体由 7892 进程拦截。主机公网 SSH 不妨碍未来限制出站公网来进行离线验收，当前并未实施该限制。
