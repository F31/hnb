# HNB Governance Framework

## Capability Tiers
| Tier | Definition | Examples |
|------|-----------|---------|
| T0 | Core kernel, always installed | Identity, Platform Kernel, Operations |
| T1 | Default delivery | Alert Notification, Gateway, Deployment Governance |
| T2 | Standard optional | Provider Conformance, Observability & DR |
| T3 | POC/Conformance gated | AI Extension, Edge Pack |

## Stage Gates
| Stage | Entry Criteria | Exit Criteria |
|-------|---------------|---------------|
| Phase 0 | Architecture baseline | Specs complete for MVP scope |
| MVP | First delivery closed loop | All P0/P1 tasks complete, e2e verified |
| V1 | MVP exit | 100% test coverage, documentation complete |
| V1.5 | V1 exit | 3 production deployments, performance baselines |
| V2 | V1.5 exit | Full DR verified, enterprise compliance |

## Definition of Done
Before archiving a change:
- [x] Specs synced to main
- [x] Code implemented
- [x] Database migrations
- [x] Automated tests pass
- [x] Documentation updated
- [x] Upgrade/rollback verified
- [x] Telemetry/observability added
- [x] Security review completed