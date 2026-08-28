# HNB Restore Runbook

## Prerequisites
- PostgreSQL 16 client tools installed
- Access to target database
- Backup file (`.sql.gz`) from backup script

## Restore Procedure

### 1. Identify the backup
```bash
ls -la backups/
# Pick the desired backup file: hnb-20260727T120000Z.sql.gz
```

### 2. Restore to a new database
```bash
# Create target database
createdb hnb-restore-test

# Restore
gunzip -c backups/hnb-20260727T120000Z.sql.gz | psql hnb-restore-test
```

### 3. Verify restoration
```bash
psql hnb-restore-test -c "SELECT count(*) FROM operations;"
psql hnb-restore-test -c "SELECT count(*) FROM provider_manifests;"
```

### 4. Rollback (if needed)
```bash
# Drop the restored database
dropdb hnb-restore-test

# Re-run restore with a different backup
```

## Recovery Time Objective (RTO)
- Target: < 30 minutes for full restore
- Measured: [record actual time here]

## Recovery Point Objective (RPO)
- Target: < 1 hour data loss
- Achieved by: hourly pg_dump cron job