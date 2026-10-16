import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from datetime import datetime, timezone

env = {**os.environ, 'KUBECONFIG': '/etc/rancher/k3s/k3s.yaml'}
def kubectl(*args, stdin=None):
    return subprocess.run(['k3s', 'kubectl', *args], input=stdin, text=True,
                          capture_output=True, env=env, check=True).stdout

deadline = time.monotonic() + 300
while True:
    pods = json.loads(kubectl('get', 'pods', '-n', 'hnb-system', '-o', 'json'))['items']
    ready = len(pods) == 10 and all(
        p['status']['phase'] == 'Running' and
        all(c['ready'] for c in p['status'].get('containerStatuses', [])) and
        p['status'].get('containerStatuses') for p in pods)
    if ready:
        break
    if time.monotonic() >= deadline:
        raise SystemExit('10 ready workloads not reached')
    time.sleep(5)
portal = next(p['metadata']['name'] for p in pods if p['metadata']['labels']['app.kubernetes.io/name'] == 'portal')

def http(url, token=None, payload=None):
    args = ['exec', '-i', '-n', 'hnb-system', portal, '--', 'curl', '-sS', '-m', '20',
            '-w', '\n%{http_code}']
    if token:
        args.extend(['-H', 'Authorization: Bearer ' + token])
    if payload is not None:
        args.extend(['-X', 'POST', '-H', 'Content-Type: application/json', '-d', '@-'])
    args.append(url)
    raw = kubectl(*args, stdin=json.dumps(payload) if payload is not None else None)
    body, code = raw.rsplit('\n', 1)
    return int(code), body

password = Path('/var/lib/hnb-startup-eval/identity/admin-bootstrap.password').read_text().strip()
code, body = http('http://portal/api/v1/auth/login', payload={'username': 'admin', 'password': password})
assert code == 200, f'login status {code}'
payload = json.loads(body)
token = payload['data']['access_token']
assert token and payload['code'] == 0
checks = {'login_via_portal': {'status': code, 'token_issued': True}}
for label, url, expected, authenticated in (
    ('apiserver_health', 'http://apiserver:8080/health', 200, False),
    ('app_market_health', 'http://app-market:8080/health', 200, False),
    ('platform_api_health_requires_auth', 'http://platform-api:8080/health', 401, False),
    ('portal_index', 'http://portal/', 200, False),
    ('anonymous_me', 'http://portal/api/v1/auth/me', 401, False),
    ('workspaces', 'http://portal/api/v1/workspaces', 200, True),
    ('tenants', 'http://portal/api/v1/tenants', 200, True),
    ('unauthorized_products', 'http://portal/api/v1/products', 403, True),
):
    code, body = http(url, token if authenticated else None)
    assert code == expected, f'{label}: expected {expected}, received {code}'
    checks[label] = {'status': code}
    if label == 'portal_index':
        assert 'HNB' in body
        checks[label]['spa_served'] = True
    if label in ('workspaces', 'tenants'):
        value = json.loads(body)
        assert value['code'] == 0 and value['data']
        checks[label]['data_sha256'] = hashlib.sha256(
            json.dumps(value['data'], sort_keys=True).encode()).hexdigest()

mem = {line.split(':')[0]: int(line.split()[1]) * 1024
       for line in Path('/proc/meminfo').read_text().splitlines() if line.split()[1].isdigit()}
disk = os.statvfs('/')
report = {
    'verified_at': datetime.now(timezone.utc).isoformat(),
    'boot_id': Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
    'source': 'current Chart rendered locally; namespace supplied for namespaced resources',
    'checks': checks,
    'pods': [{'name': p['metadata']['name'], 'phase': p['status']['phase'],
              'ready': all(c['ready'] for c in p['status']['containerStatuses']),
              'restarts': sum(c['restartCount'] for c in p['status']['containerStatuses']),
              'image': p['spec']['containers'][0]['image']} for p in pods],
    'node': {'memory_total_bytes': mem['MemTotal'], 'memory_available_bytes': mem['MemAvailable'],
             'disk_total_bytes': disk.f_blocks * disk.f_frsize,
             'disk_used_bytes': (disk.f_blocks - disk.f_bfree) * disk.f_frsize,
             'disk_available_bytes': disk.f_bavail * disk.f_frsize},
}
out = Path('/home/ubuntu') / ('current-verification-' + sys.argv[1] + '.json')
out.write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report, indent=2))
