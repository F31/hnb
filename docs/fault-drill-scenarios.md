# Fault Drill Scenarios

## Lite HA
| Scenario | Procedure | Expected Outcome |
|----------|-----------|-----------------|
| Single Pod failure | `kubectl delete pod -l app=hnb-api` | New pod starts, API continues serving |
| Read replica failure | Stop read replica PostgreSQL | Writes continue, reads fall back to primary |

## Standard HA
| Scenario | Procedure | Expected Outcome |
|----------|-----------|-----------------|
| Primary DB failure | `kubectl delete pod -l role=primary` | Automatic failover to replica within 30s |
| NATS node failure | Stop one NATS node | Messages re-routed to remaining nodes |
| Provider crash | Stop provider process | Operation timed out, no crash propagation |

## Enterprise
| Scenario | Procedure | Expected Outcome |
|----------|-----------|-----------------|
| Multi-AZ failure | Simulate AZ outage via network policy | Cross-AZ traffic continues |
| Full DR failover | Restore backup to secondary region | RPO < 1h, RTO < 30min |