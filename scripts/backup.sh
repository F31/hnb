#!/bin/bash
# HNB Backup Script
# Usage: ./scripts/backup.sh <db-name> <output-dir>

set -euo pipefail

DB_NAME="${1:-hnb}"
OUTPUT_DIR="${2:-./backups}"
TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
BACKUP_FILE="${OUTPUT_DIR}/${DB_NAME}-${TIMESTAMP}.sql.gz"
LATEST_LINK="${OUTPUT_DIR}/${DB_NAME}-latest.sql.gz"

mkdir -p "$OUTPUT_DIR"

echo "Backing up ${DB_NAME} to ${BACKUP_FILE}..."
pg_dump --clean --if-exists --no-owner --no-acl "$DB_NAME" | gzip > "$BACKUP_FILE"

# Update latest symlink
ln -sf "$(basename "$BACKUP_FILE")" "$LATEST_LINK"

echo "Backup complete: $(du -h "$BACKUP_FILE" | cut -f1)"
echo "To restore: gunzip -c ${BACKUP_FILE} | psql ${DB_NAME}"