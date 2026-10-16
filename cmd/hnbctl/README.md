# hnbctl

HNB 部署交付与纳管命令行工具。

```bash
go build -o hnbctl .
./hnbctl --help
./hnbctl os
./hnbctl os inspect --format json
./hnbctl preflight -f hnb.yaml --ask-pass
./hnbctl ssh enroll -f hnb.yaml --node control-plane-1 --key-file ./credentials/control-plane-1 --ask-pass
./hnbctl install -f hnb.yaml
./hnbctl install --host 192.0.2.10 --user ubuntu --ask-pass --domain hnb.example.com --tls-secret hnb-tls --storage-class hnb-topolvm --lock release.lock.yaml --plan
./hnbctl status --watch
./hnbctl history
./hnbctl logs --tail 50
./hnbctl uninstall -f hnb.yaml
./hnbctl uninstall --installation <runId> --plan
```

极简参数自动生成并保存配置；当前 `install` / `uninstall` 仍为计划与部分只读预检，
不会实际完成平台安装或删除。卸载默认保留数据，删除数据需显式 `--delete-data`。

命令范围、版本化 JSON、退出码与 OS 适配边界见
[CLI 接口说明](../../docs/deployment/hnbctl-interface.md)。
Ubuntu 26.04 的现有 K3s 路线真机证据见
[适配与部署复验](../../docs/acceptance/reports/20261015-ubuntu2604-adaptation.md)。
