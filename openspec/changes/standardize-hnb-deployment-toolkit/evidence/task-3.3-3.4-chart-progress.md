# 3.3/3.4 新组件 Chart 进展

- tasks: 3.3, 3.4
- requirements: PINST-002, PINST-005, DBND-001
- date: 2026-10-11
- status: partial（均保持未勾选）

## 本轮交付

父 Chart 声明 Portal、plugin-worker、runtime-target-lifecycle-provider 依赖。
默认关闭候选组件；显式启用须提供真实 digest 与所需 Secret 引用。
三个新子 Chart 添加 digest/Schema，渲染使用 `repository@sha256:...`，
缺失、格式错误及全零占位摘要均拒绝。渲染检查不验证 registry 内容。

- Portal：Deployment/Service/可选 TLS Ingress；ConfigMap 挂载实际 Nginx 配置，
  使用镜像的 `/usr/share/nginx/html/shell` 与 `/modules/` 布局；同源 `/api/`、
  `/tunnel` 代理到明确内部 API origin。删除镜像不会读取的 `HNB_API_BASE_URL`。
  ConfigMap checksum 触发配置变化后的滚动更新；启用 Ingress 必须提供 host/TLS Secret。
- plugin-worker：数据库和 KMS 使用 Secret 引用；NATS 证书挂载配套设置
  `NATS_TLS_CERT_FILE`、`NATS_TLS_KEY_FILE`、`NATS_TLS_CA_FILE`，与真实消息客户端约定一致。
- lifecycle provider：固定 Kubernetes profile 与 `LISTEN_ADDRESS=:18082`，
  添加内部 Service 和 `/healthz` readiness；DB/KMS 使用 Secret 引用。
  删除进程不消费的 NATS 环境变量和证书挂载。

## 验证与范围

```bash
python3 deploy/charts/hnb/test_candidate_components.py
```

Helm `v4.2.4+g3900f43`；四个 unittest 通过，覆盖三个子 Chart 的 lint/template、
digest 拒绝、Secret/TLS 接线、Provider profile/端口、Portal 服务/入口与代理配置。
重复字符摘要、fixture Secret 名和 `.invalid` 域名仅用于渲染测试，不能当作发布锁或认证。

## 未完成项

- 3.3：既有组件仍使用 tag/global.imageTag（含 latest 默认值），尚未做到父 Chart 全量镜像锁定；
  Portal 镜像构建/实际入口登录未验收。
- 3.4：`cmd/runtime-target-lifecycle-provider/internal/provider/http.go` 的 Handler 当前
  直接开放 `/v2/steps:execute`，`main.go` 未装配服务身份校验；仅添加不存在的环境变量不能解决。
  owning worker 的 authenticated route 与 provider verifier 需按既有身份契约补齐并负向测试。
- 未执行容器启动、完整父 Chart 安装或平台 E2E；不以子 Chart 渲染替代以上证据。
