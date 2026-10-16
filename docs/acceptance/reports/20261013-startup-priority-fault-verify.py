import json
from pathlib import Path
import subprocess
import time
from datetime import datetime, timezone

def kubectl(*args):
    return subprocess.check_output(['k3s', 'kubectl', '-n', 'hnb-system', *args], text=True)
def pods():
    return json.loads(kubectl('get', 'pods', '-o', 'json'))['items']
def wait_ready():
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        current = pods()
        if len(current) == 10 and all(p.get('status', {}).get('containerStatuses')
                                     and all(c['ready'] for c in p['status']['containerStatuses']) for p in current):
            return current
        time.sleep(2)
    raise RuntimeError('10 ready workloads not reached')

before = wait_ready()
portal = next(p['metadata']['name'] for p in before if p['metadata']['labels']['app.kubernetes.io/name'] == 'portal')
core = {p['metadata']['labels']['app.kubernetes.io/name']: p for p in before
        if p['metadata']['labels']['app.kubernetes.io/name'] in
        ('apiserver', 'app-market', 'platform-api', 'operation-worker', 'extension-controller')}
for p in before:
    for c in p['spec']['containers']:
        for env in c.get('env', []):
            if env['name'] == 'DB_DSN':
                assert 'value' not in env and 'secretKeyRef' in env['valueFrom']

def http(pod, path):
    port = 8081 if pod['metadata']['labels']['app.kubernetes.io/name'] in ('operation-worker', 'extension-controller') else 8080
    url = f"http://{pod['status']['podIP']}:{port}{path}"
    return int(kubectl('exec', portal, '--', 'curl', '-sS', '-m', '5', '-o', '/dev/null', '-w', '%{http_code}', url))
def restarts(current):
    return {p['metadata']['name']: sum(c['restartCount'] for c in p['status']['containerStatuses'])
            for p in current if p['metadata']['labels']['app.kubernetes.io/name'] in core}

report = {'started_at': datetime.now(timezone.utc).isoformat(), 'assertions': {}, 'outages': {}}
for name, p in core.items():
    assert http(p, '/livez') == 204 and http(p, '/readyz') == 204
report['assertions']['database_credentials_are_secret_refs'] = True
report['assertions']['five_core_health_endpoints_ready'] = True

for backing, affected in [('nats', set(core) - {'platform-api'}), ('postgresql', set(core))]:
    original = restarts(wait_ready())
    try:
        kubectl('scale', 'sts', backing, '--replicas=0')
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if not any(p['metadata']['name'] == backing + '-0' for p in pods()):
                break
            time.sleep(1)
        else:
            raise RuntimeError('dependency pod did not terminate')
        time.sleep(12)
        statuses = {name: {'live': http(p, '/livez'), 'ready': http(p, '/readyz')} for name, p in core.items()}
        for name in affected:
            assert statuses[name] == {'live': 204, 'ready': 503}, (backing, name, statuses[name])
        assert restarts(pods()) == original, 'dependency loss restarted a core service'
        report['outages'][backing] = statuses
    finally:
        kubectl('scale', 'sts', backing, '--replicas=1')
        wait_ready()
    for name, p in core.items():
        assert http(p, '/readyz') == 204
    assert restarts(pods()) == original, 'recovery restarted a core service'
    report['assertions'][backing + '_outage_drops_readiness_without_restart'] = True
    report['assertions'][backing + '_recovery_is_automatic'] = True

after = wait_ready()
report['pods'] = [{'name': p['metadata']['name'], 'ready': all(c['ready'] for c in p['status']['containerStatuses']),
                  'restarts': sum(c['restartCount'] for c in p['status']['containerStatuses'])} for p in after]
report['finished_at'] = datetime.now(timezone.utc).isoformat()
report['exit_code'] = 0
Path('/home/ubuntu/priority-fault-verification.json').write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report, indent=2))
