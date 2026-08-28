# HNB Cloud Native Platform

HNB 是一个云原生运维平台，采用微内核 + Provider 架构，通过 Operation Engine 实现统一的运维操作编排与执行。

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Portal (TBD)                         │
├─────────────────────────────────────────────────────────────┤
│                     Platform API (:8080)                     │
│          REST API · Validation · Tenant Isolation           │
├──────────────────────┬──────────────────────────────────────┤
│    PostgreSQL Outbox  │         NATS JetStream              │
│    (Transactional)    │         (Async Events)              │
├──────────────────────┴──────────────────────────────────────┤
│                     Operation Worker                        │
│        10-State Machine · DAG Orchestration · Fencing       │
├──────────┬──────────┬──────────┬──────────┬─────────────────┤
│ K8s      │ Edge     │ Gateway  │ App      │ RBAC            │
│ Provider │ Provider │ Provider │ Market   │ Syncer          │
│ (:18080) │ (:18081) │ (NATS)   │ (NATS)   │ (:8080/:8081)   │
└──────────┴──────────┴──────────┴──────────┴─────────────────┘
```

### Services

| Service | Port | Protocol | Dependencies | Description |
|---------|------|----------|-------------|-------------|
| **platform-api** | 8080 | HTTP/REST | PostgreSQL | 运维操作 API 入口，提交/审批/查询 |
| **operation-worker** | — | NATS | PostgreSQL, NATS | 操作状态机引擎，DAG 编排，Outbox 消费 |
| **kubernetes-provider** | 18080 | HTTP | Kubernetes API | K8s 资源管理（Deploy/Scale/Rollback） |
| **edge-provider** | 18081 | HTTP | CloudCore/KubeEdge | 边缘节点资源管理 |
| **gateway-provider** | — | NATS | PostgreSQL, NATS | 网关配置管理 |
| **app-market** | — | NATS | PostgreSQL, NATS | 应用市场管理 |
| **rbac-syncer** | 8080/8081 | HTTP | PostgreSQL, K8s API | 平台 RBAC → K8s RBAC 同步 |

## Prerequisites

- Go 1.24+
- PostgreSQL 16+
- NATS JetStream 2.10+
- Docker (for container builds)

## Quick Start

```bash
# 1. Clone and enter workspace
git clone <repo>
cd hnb

# 2. Build all services
make build

# 3. Run all unit tests
make test

# 4. Run all linters (go vet)
make lint

# 5. Build Docker images (requires Docker)
make docker
```

## Database Setup

```bash
# Create test database
createdb hnb

# Apply forward migrations explicitly. The runner excludes every *.rollback.sql
# and stops on the first SQL error.
DATABASE_URL=postgresql:///hnb bash database/postgresql/scripts/migrate.sh

# Verify empty, repeat, legacy-upgrade, rollback, and recovery paths in an
# isolated temporary postgres:16 container.
bash database/postgresql/scripts/test-migrations.sh
```

Docker Compose does not auto-run SQL from `/docker-entrypoint-initdb.d`.
After the `postgres` service is healthy, run the same explicit migration command
against `postgresql://hnb:hnb123@127.0.0.1:5432/hnb`.

## Integration Tests

Integration tests are gated by `HNB_TEST_POSTGRES_DSN` environment variable. They are
automatically skipped when not set.

```bash
# Run platform-api integration tests
make integration-test-platform-api

# Run operation-worker integration tests
make integration-test-operation-worker

# Run all integration tests
make integration-test
```

## Project Structure

```
hnb/
├── cmd/                          # Service entrypoints (Go modules)
│   ├── platform-api/             # REST API service
│   ├── operation-worker/         # Operation state machine engine
│   ├── kubernetes-provider/      # K8s runtime provider
│   ├── edge-provider/            # Edge runtime provider
│   ├── gateway-provider/         # Gateway config provider
│   ├── app-market/               # App market service
│   └── rbac-syncer/              # RBAC sync service
├── contracts/                    # Schema-first contracts
│   ├── openapi/                  # OpenAPI 3.1 specs
│   ├── proto/                    # Protobuf specs
│   ├── schema/                   # JSON Schema specs
│   ├── generated/                # Generated code (Go, TypeScript)
│   └── mappings/                 # Event envelope mappings
├── database/
│   └── postgresql/
│       ├── migrations/           # 16 sequential migrations
│       └── rollbacks/            # Rollback scripts
├── deploy/                       # Deployment manifests
│   ├── docker-compose/           # Local dev compose files
│   └── helm/                     # Helm charts (per service)
├── openspec/                     # OpenSpec governance
│   ├── specs/                    # Domain specs (18 domains)
│   ├── changes/                  # Active changes (7 items)
│   │   └── archive/              # Archived changes (7 items)
│   └── architecture.md           # Architecture decision records
├── scripts/                      # Tooling scripts
│   ├── validate-contracts.mjs    # Contract validation gate
│   ├── validate-openspec.mjs     # OpenSpec validation gate
│   ├── generate-contracts.mjs    # Code generation
│   └── bootstrap-contract-tools.mjs
├── go.work                       # Go workspace (all modules)
├── Makefile                      # Top-level build orchestration
└── .github/workflows/ci.yml      # GitHub Actions CI
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build all 7 services |
| `make test` | Run all unit tests |
| `make lint` | Run `go vet` on all modules |
| `make docker` | Build Docker images for all services |
| `make integration-test` | Run PostgreSQL integration tests |
| `make build-<service>` | Build a single service (e.g., `make build-platform-api`) |
| `make test-<service>` | Test a single service |
| `make clean` | Remove build artifacts |

## CI/CD

The CI pipeline (`.github/workflows/ci.yml`) runs on push/PR to `main`:

1. **Lint**: `go vet` + OpenSpec validation + Contract validation
2. **Build**: `go build` for all 7 modules
3. **Unit Test**: `go test -race` for all 7 modules
4. **Integration Test**: PostgreSQL 16 container + migrations + PG store tests

## OpenSpec Governance

HNB 使用 OpenSpec 进行规格驱动开发。每个变更对应一个 `openspec/changes/<name>/` 目录，
包含 proposal、design、tasks、evidence 和 specs。

```bash
# Validate all specs
node scripts/validate-openspec.mjs

# Validate contracts
npm run contracts:validate

# Bootstap contract tools
npm run contracts:bootstrap
```

## Operator Documentation

- [Operator documentation index](docs/README.md)
- [Workload storage operator guide](docs/workload-storage-operator-guide.md)

## License

Internal project.
