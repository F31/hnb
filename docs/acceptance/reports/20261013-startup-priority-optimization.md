# Startup 优先级优化：Secret、启动可靠性与锁定渲染

- run_id: `ubuntu-startup-priority-20261013`
- host: `106.75.34.133`；Ubuntu 24.04，2 vCPU / 3914Mi / 根盘 18.33GiB
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73`，工作区含本次优化；源文件与产物摘要见 artifacts JSON
- started_at: `2026-10-13T09:17:56.438153+00:00`
- finished_at: `2026-10-13T09:22:23.434524+00:00`
- exit_code: `0`（故障演练与重启业务断言）
- 结论：**当前 Startup 10/10 Ready；PG/NATS 故障仅影响 readiness，无核心进程重启；整机重启后正常恢复，每个容器重启次数仅增加一次。**

## 范围与环境条件

本轮在已运行的单节点 K3s `v1.36.5+k3s1` 上升级 Startup 初始组合，保留现有数据库、
身份、TLS、PVC 和已导入镜像缓存。不是干净 OS/全新集群/冷缓存首次安装，也不是完整
Minimal、真实 Operation/Agent、HA 或离线认证。首轮测试见
`20261013-startup-composition-deploy.md`，本轮保留其历史证据。

CLI 新增 `hnbctl render`，完成锁定候选物料的可重复渲染；`hnbctl install` 的完整实际
执行仍未开放，独立 Minimal E2E/真实 Operation/Agent/发布物料资格尚缺。

## 优化内容

1. 17 个旧数据库客户端模板新增 `db.existingSecret`/`db.secretKey`，与已有两个 Secret
   客户端统一。优先 `secretKeyRef`；旧 DSN 输入仍兼容。真机 platform/market owner
   Secret 增加 `dsn` 键，保留原有 `password`，六个 Startup DB_DSN 均不再内联于 Pod spec。
2. apiserver/app-market/platform-api/operation-worker/extension-controller 使用数据库有界
   退避等待；NATS 对无可用服务/超时进行有界启动重试。两种依赖分别最多等待两分钟，
   不把配置/凭据错误当作临时网络故障。
3. 五个核心服务增加 `/livez`、`/readyz` 和独立 startup/readiness/liveness probe。
   worker/controller 使用 8081 健康端口；只将依赖故障用于 readiness。
4. `hnbctl render` 核验 Chart 包摘要、包内 overlay、lock 中 amd64 镜像，绑定 Secret/
   namespace；拒绝内联 DB_DSN、未锁定镜像和跨 namespace 引用。同路径同内容可重放，
   不覆盖不同内容。其输出明确 `installationExecuted: false`。
5. 修正已有集群安装计划顺序：基础设施 → owner 迁移 → 平台服务。

## 执行命令

本机先构建当前服务/CLI，打包当前 Chart，以实际 archive/CLI/amd64 镜像摘要生成候选锁。
没有把评估锁提升为认证发布锁。

```bash
hnbctl render -f priority-config.yaml \
  --chart-package hnb-0.1.0.tgz --bindings priority-bindings.yaml \
  --stage startup --output priority-rendered.yaml
# 同命令再次执行：退出 0，输出内容一致。

sudo -n python3 /home/ubuntu/resolve-images.py
sudo -n python3 /home/ubuntu/prepare-db-secrets.py
sudo -n k3s kubectl apply -f /home/ubuntu/priority-rendered.yaml
sudo -n python3 /home/ubuntu/verify-priority.py
sudo -n python3 /home/ubuntu/verify-current.py priority-before-reboot
sudo -n systemctl reboot
sudo -n python3 /home/ubuntu/verify-current.py priority-after-reboot
```

滚动更新期间对旧/新 Pod 集合运行 `kubectl wait --all` 遇到旧 Pod 删除的 NotFound。
随后验证脚本按实际集合等待恰好 10 个 Ready Pod，通过后再进行演练；未把该 wait
错误当作部署成功证据。

## 真机故障演练

依次将本次评估的 NATS、PostgreSQL StatefulSet 副本数降至 0，等待退出，再观察 12 秒。
脚本在 `finally` 中恢复副本数为 1，并等待 10 个工作负载重新 Ready。

| 场景 | `/livez` | `/readyz` | 核心服务重启 |
|---|---|---|---|
| NATS 停止：apiserver/app-market/worker/controller | 204 | 503 | 无 |
| NATS 停止：platform-api（实际不使用 NATS） | 204 | 204 | 无 |
| PostgreSQL 停止：五个核心服务 | 204 | 503 | 无 |
| 各依赖恢复 | 204 | 204 | 无，自动恢复 |

演练后 Portal 代理登录、工作空间/租户读取、匿名 401、无权限路由 403 均通过。
这证明连接和就绪恢复，不证明所有异步订阅、真实 Operation 或消息消费语义已经验收。

## 重启复验

- boot_id：`2be57f51-32ee-46b3-b830-f7637c8b320a` → `a40da539-4d66-465f-a544-1c9dfb1ddb3a`。
- 10/10 Ready，登录、健康、鉴权及数据读取断言通过。
- 工作空间和租户数据 SHA-256 与重启前一致。
- 每个容器的 restartCount 相比重启前恰好 +1；五个核心服务没有依赖启动竞态导致的额外重启。
- 重启后可用内存约 2948MiB，根盘已用约 5.50GiB，可用约 12.82GiB。

## 自动化检查

- `python3 -m unittest test_candidate_components test_profiles`：47 项通过。
- `go test -race`：pkg/db、pkg/messaging、apiserver、app-market、platform-api、operation-worker、extension-controller 通过。
- hnbctl `go test -race`：CLI/toolkit 通过，含真实 Helm 渲染与输出重放/拒绝测试。
- `openspec validate --all --strict`：29 项通过；规格质量门禁 31 specs / 242 requirements / 377 scenarios 通过。
- `git diff --check`：通过。

## 连接条件与失败记录

上传期间本地网络同步曾重置直连路由，流量回到代理虚拟接口 eth5，造成超时/连接关闭。
清除 HTTP(S)/ALL_PROXY 环境变量、SSH `-F /dev/null`、禁用 ProxyCommand/ProxyJump 后，
仍需在传输期间保持仅面向该主机的 eth2 专用路由。采用该方式后上传、演练及重启复验
完成；未关闭系统代理或更换云主机。上传失败没有作为平台故障或冷缓存安装失败计入。

## 保留证据

- `20261013-startup-priority-fault-verify.py`
- `20261013-startup-priority-fault-verification.json`
- `20261013-startup-priority-before-reboot.json`
- `20261013-startup-priority-after-reboot.json`
- `20261013-startup-priority-artifacts.json`

Chart、CLI、镜像、渲染清单及源文件身份记录于 artifacts JSON；旧含内联 DSN 的评估
清单留在受限临时/主机目录，不复制进证据。
