BEGIN;
ALTER TABLE runtime_intents
    DROP COLUMN IF EXISTS response_http_status,
    DROP COLUMN IF EXISTS accepted_status,
    DROP COLUMN IF EXISTS commitment_action;
COMMIT;
