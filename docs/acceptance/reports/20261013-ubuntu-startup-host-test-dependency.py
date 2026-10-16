# Dependency-only real-host experiment. Run stages once on a dedicated Ubuntu
# node as root: seed, backup, reboot, verify, restore. Not a platform installer.
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import time
import uuid

ROOT = Path('/var/lib/hnb-cloud-drill-20261013')
PLANES = ('platform', 'market', 'artifact')
PAYLOAD = b'hnb-owned-cloud-drill-20261013'

def run(*args):
    return subprocess.check_output(args, stderr=subprocess.STDOUT).decode().strip()

def sql(db, query):
    return run('sudo', '-u', 'postgres', 'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-At', '-d', db, '-c', query)

def request(subject, body):
    with socket.create_connection(('127.0.0.1', 14222), timeout=10) as s:
        s.settimeout(10)
        f = s.makefile('rb')
        assert f.readline().startswith(b'INFO ')
        inbox = '_INBOX.' + uuid.uuid4().hex
        data = json.dumps(body).encode() if isinstance(body, dict) else body
        s.sendall(b'CONNECT {"verbose":false,"pedantic":false}\r\n')
        s.sendall(('SUB ' + inbox + ' 1\r\nPUB ' + subject + ' ' + inbox + ' ' + str(len(data)) + '\r\n').encode() + data + b'\r\n')
        while True:
            line = f.readline()
            if line == b'PING\r\n':
                s.sendall(b'PONG\r\n')
            elif line.startswith(b'MSG '):
                size = int(line.split()[-1])
                payload = f.read(size)
                assert f.read(2) == b'\r\n'
                result = json.loads(payload)
                assert 'error' not in result, result
                return result
            elif not line:
                raise RuntimeError('NATS connection closed')

def verify(suffix=''):
    for plane in PLANES:
        assert sql('hnb_drill_' + plane + suffix, 'SELECT id || ":" || payload FROM drill ORDER BY id'.replace('":"', "':'")) == '1:' + PAYLOAD.decode()
    msg = request('$JS.API.STREAM.MSG.GET.HNB_DRILL', {'seq': 1})['message']
    assert base64.b64decode(msg['data']) == PAYLOAD
    assert request('$JS.API.STREAM.INFO.HNB_DRILL', {})['state']['messages'] == 1
    expected = json.loads((ROOT / 'expected.json').read_text())
    for name in ('identity', 'kms'):
        path = ROOT / name
        assert hashlib.sha256(path.read_bytes()).hexdigest() == expected[name]
        assert path.stat().st_mode & 0o077 == 0
    print(json.dumps({'action': 'verify', 'databaseSuffix': suffix, 'pgPlanes': 3, 'jetstreamMessages': 1, 'identityKmsDigestMatch': True, 'bootId': Path('/proc/sys/kernel/random/boot_id').read_text().strip()}))

action = sys.argv[1]
if action == 'seed':
    assert not ROOT.exists(), 'refuse existing experiment directory'
    ROOT.mkdir(mode=0o700)
    (ROOT / 'nats').mkdir(mode=0o700)
    expected = {}
    for name in ('identity', 'kms'):
        path = ROOT / name
        path.write_bytes(os.urandom(32))
        path.chmod(0o600)
        expected[name] = hashlib.sha256(path.read_bytes()).hexdigest()
    (ROOT / 'expected.json').write_text(json.dumps(expected))
    (ROOT / 'expected.json').chmod(0o600)
    # The distro service was installed only for this experiment. Use an isolated
    # loopback listener and bounded JetStream store for the actual drill.
    run('systemctl', 'disable', '--now', 'nats-server')
    conf = ROOT / 'nats.conf'
    conf.write_text('host: 127.0.0.1\nport: 14222\njetstream { store_dir: "' + str(ROOT / 'nats') + '", max_memory_store: 64MB, max_file_store: 256MB }\n')
    conf.chmod(0o600)
    unit = Path('/etc/systemd/system/hnb-cloud-drill-nats.service')
    assert not unit.exists()
    unit.write_text('[Unit]\nAfter=network.target\n[Service]\nExecStart=/usr/sbin/nats-server -c ' + str(conf) + '\nRestart=on-failure\n[Install]\nWantedBy=multi-user.target\n')
    run('systemctl', 'daemon-reload')
    run('systemctl', 'enable', '--now', 'hnb-cloud-drill-nats')
    for _ in range(30):
        try:
            request('$JS.API.INFO', {})
            break
        except (OSError, AssertionError):
            time.sleep(1)
    for plane in PLANES:
        role = 'hnb_drill_' + plane
        sql('postgres', 'CREATE ROLE ' + role + ' NOLOGIN')
        run('sudo', '-u', 'postgres', 'createdb', '-O', role, role)
        sql(role, 'SET ROLE ' + role + '; CREATE TABLE drill (id integer PRIMARY KEY, payload text NOT NULL); INSERT INTO drill VALUES (1, \'' + PAYLOAD.decode() + '\')')
    request('$JS.API.STREAM.CREATE.HNB_DRILL', {'name': 'HNB_DRILL', 'subjects': ['hnb.drill'], 'storage': 'file', 'num_replicas': 1})
    assert request('hnb.drill', PAYLOAD)['seq'] == 1
    verify()
elif action == 'verify':
    verify()
elif action == 'backup':
    backup = ROOT / 'backup'
    backup.mkdir(mode=0o700)
    for plane in PLANES:
        data = subprocess.check_output(['sudo', '-u', 'postgres', 'pg_dump', '-Fc', 'hnb_drill_' + plane])
        path = backup / (plane + '.dump')
        path.write_bytes(data)
        path.chmod(0o600)
    run('systemctl', 'stop', 'hnb-cloud-drill-nats')
    try:
        shutil.copytree(ROOT / 'nats', backup / 'nats')
    finally:
        run('systemctl', 'start', 'hnb-cloud-drill-nats')
    for name in ('identity', 'kms'):
        shutil.copy2(ROOT / name, backup / name)
    inventory = {str(p.relative_to(backup)): hashlib.sha256(p.read_bytes()).hexdigest() for p in backup.rglob('*') if p.is_file()}
    (ROOT / 'backup-digests.json').write_text(json.dumps(inventory, sort_keys=True))
    print(json.dumps({'action': 'backup', 'files': len(inventory), 'pgFormat': 'custom', 'jetstreamMethod': 'quiesced-store-copy', 'snapshotAt': time.time()}))
elif action == 'restore':
    start = time.monotonic()
    backup = ROOT / 'backup'
    inventory = json.loads((ROOT / 'backup-digests.json').read_text())
    for name, digest in inventory.items():
        assert hashlib.sha256((backup / name).read_bytes()).hexdigest() == digest
    for plane in PLANES:
        db = 'hnb_drill_' + plane + '_restored'
        run('sudo', '-u', 'postgres', 'createdb', '-O', 'hnb_drill_' + plane, db)
        with (backup / (plane + '.dump')).open('rb') as f:
            subprocess.run(['sudo', '-u', 'postgres', 'pg_restore', '--exit-on-error', '-d', db], stdin=f, check=True)
    request('$JS.API.STREAM.DELETE.HNB_DRILL', {})
    run('systemctl', 'stop', 'hnb-cloud-drill-nats')
    shutil.rmtree(ROOT / 'nats')
    shutil.copytree(backup / 'nats', ROOT / 'nats')
    run('systemctl', 'start', 'hnb-cloud-drill-nats')
    for name in ('identity', 'kms'):
        (ROOT / name).write_bytes(b'controlled-test-corruption')
        shutil.copy2(backup / name, ROOT / name)
    verify('_restored')
    print(json.dumps({'action': 'restore', 'measuredSeconds': round(time.monotonic() - start, 3), 'approvedRtoRpoVerified': False, 'platformCertified': False}))
else:
    raise RuntimeError('unknown action')
