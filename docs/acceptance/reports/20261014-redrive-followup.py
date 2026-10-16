"""Evaluation-only duplicate delivery and archived cleanup of known test artifacts."""
import importlib.util
import json
import os
import time
from pathlib import Path

spec = importlib.util.spec_from_file_location('recovery', Path(__file__).with_name('20261014-redrive-recovery.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

os.umask(0o077)
operation = json.loads(m.RESULT.read_text())['operation_id']
before = m.snapshot(operation)
assert before['status'] == 'succeeded'
# Republish after the command stream's two-minute duplicate window, so the
# worker itself (rather than broker deduplication) must reject the duplicate.
wait = float(m.q("select greatest(0, 125-extract(epoch from now()-published_at))"
                 " from outbox_events where operation_id='%s'"
                 " and message_type='hnb.command.operation.step-requested.v1'" % operation))
time.sleep(wait)
changed = m.q("update outbox_events set status='pending', attempt=attempt+1, next_attempt_at=now()"
              " where operation_id='%s' and message_type='hnb.command.operation.step-requested.v1'"
              " and status='published'" % operation)
assert changed == 'UPDATE 1', changed
deadline = time.monotonic() + 30
while time.monotonic() < deadline:
    after = m.snapshot(operation)
    if after['requests'][0]['status'] == 'published':
        break
    time.sleep(1)
logs = m.k('logs', 'deployment/operation-worker', '--since=2m')
stale = [line for line in logs.splitlines() if operation in line and 'acknowledging stale command' in line]
assert stale, 'no worker acknowledgment of the duplicate command'
assert before['steps'] == after['steps'], 'duplicate changed execution facts'
assert after['completed_steps'] == 1 and after['status'] == 'succeeded'
assert len(after['requests']) == 1 and after['requests'][0]['attempt'] == 2

# Explicit allowlist of interrupted redrive test Operations, never arbitrary data.
legacy_operations = ['0c4711d6-5c92-4c97-8d83-90cf3e8818dc',
                     '1335a418-dbc7-47d9-aed6-6e536238ae8f',
                     '17a63c7a-558a-4e41-bd8c-6e0dc55117c3',
                     '3657edc2-5f15-4e30-8113-3c00074c5d6d']
ids = ','.join("'%s'" % value for value in legacy_operations)
condition = ("e.operation_id in (%s) and e.step_id=s.id"
             " and e.message_type='hnb.command.operation.step-requested.v1'"
             " and e.idempotency_key like s.idempotency_key || '#redrive-%%'"
             " and e.status='published'" % ids)
archive = json.loads(m.q("select coalesce(json_agg(row_to_json(e)), '[]'::json)"
                         " from outbox_events e, operation_steps s where " + condition))
archive_path = Path('/home/ubuntu/redrive-v4-legacy-outbox-archive.json')
with archive_path.open('x') as stream:
    json.dump(archive, stream, indent=2)
removed = m.q('delete from outbox_events e using operation_steps s where ' + condition)
result = {'operation_id': operation, 'duplicate_wait_seconds': wait,
          'before': before, 'after': after, 'duplicate_acknowledgments': stale,
          'legacy_archive_count': len(archive), 'legacy_removed': removed,
          'legacy_archive_path': str(archive_path),
          'legacy_operation_states': {value: m.snapshot(value) for value in legacy_operations},
          'exhaustion_logs': [line for line in m.k('logs', 'deployment/operation-worker', '--since=10m').splitlines()
                              if 'exhausted its delivery budget' in line],
          'finished_at': m.stamp(), 'passed': True}
Path('/home/ubuntu/redrive-v4-followup.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result, indent=2))
