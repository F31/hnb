## ADDED Requirements

### Requirement: [MKT-004-boundary] 市场不保存运行凭据
平台内核 SHALL 拒绝接受包含明文运行凭据的 ReleaseManifest；所有 SecretReference 由平台侧 SecretResolver 解析。ReleaseManifest 中包含的 SecretReference 仅限平台内核可解析，市场无权解密。

#### Scenario: 市场提交含 SecretReference 的 Manifest
- **GIVEN** ReleaseManifest 包含 database_password: "ref://secrets/db-password"
- **WHEN** 市场提交该 Manifest
- **THEN** 市场存储的是 ref 并非明文
- **AND** 仅平台 Operation Engine 在步骤执行时可解析该值
