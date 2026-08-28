-- Rollback: 057_node_identity_source_uid
-- Description: Restore the legacy (target_id, name) uniqueness constraint.
--              Nodes with duplicate or empty names are quarantined before the
--              constraint is reinstated rather than guessed.

BEGIN;

INSERT INTO runtime_target_projection_quarantine (source_table, source_row_id, reason, detail)
SELECT 'runtime_target_nodes', n.id::text, 'duplicate_or_empty_node_name',
       jsonb_build_object('targetId', n.target_id, 'name', n.name)
FROM runtime_target_nodes n
JOIN runtime_target_nodes other ON other.target_id = n.target_id AND other.name = n.name AND other.id <> n.id
WHERE n.name IS NULL OR n.name = ''
ON CONFLICT DO NOTHING;

DELETE FROM runtime_target_nodes
WHERE (name IS NULL OR name = '')
  AND id IN (
    SELECT id FROM runtime_target_nodes
    WHERE (name IS NULL OR name = '')
  );

ALTER TABLE runtime_target_nodes ADD CONSTRAINT runtime_target_nodes_target_id_name_key
    UNIQUE (target_id, name);

COMMIT;
