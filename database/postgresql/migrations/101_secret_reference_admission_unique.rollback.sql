-- 101 rollback: remove the unique admission index only.
BEGIN;

DROP INDEX IF EXISTS uq_secret_references_admission;

COMMIT;