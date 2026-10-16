"""Real Ubuntu host CLI + dependency drill; no full-platform certification.

Run from the repository root with SSHPASS set (never written to evidence):
python3 docs/acceptance/reports/20261013-ubuntu-startup-host-test.py \
  --host <ip> --binary /tmp/opencode/hnbctl-cloud-test --reboot
Installs distro PostgreSQL/NATS on a fresh host, uses loopback-only test data,
and retains public reports locally. Refuses an existing remote experiment.
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import shlex
import subprocess
import sys
import time
from datetime import datetime, timezone


def now():
    return datetime.now(timezone.utc).isoformat()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--host', required=True)
    parser.add_argument('--binary', required=True)
    parser.add_argument('--reboot', action='store_true')
    args = parser.parse_args()
    if not os.environ.get('SSHPASS'):
        parser.error('SSHPASS is required')
    root = Path(__file__).resolve().parents[3]
    directory = Path(__file__).resolve().parent
    prefix = directory / '20261013-ubuntu-startup-host-test'
    log_path = Path(str(prefix) + '-output.txt')
    data_path = Path(str(prefix) + '-result.json')
    if log_path.exists() or data_path.exists():
        parser.error('refuse overwriting retained evidence')
    remote = '/home/ubuntu/hnb-startup-host-test-20261013'
    target = 'ubuntu@' + args.host
    options = ['-o', 'StrictHostKeyChecking=accept-new', '-o',
               'UserKnownHostsFile=/tmp/opencode/hnb-new-host-known-hosts',
               '-o', 'ConnectTimeout=12', '-o', 'KexAlgorithms=curve25519-sha256',
               '-o', 'PreferredAuthentications=password', '-o', 'PubkeyAuthentication=no']
    env = dict(os.environ)
    for name in ('http_proxy', 'https_proxy', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'all_proxy'):
        env.pop(name, None)
    result = {'run_id': 'ubuntu-startup-host-test-20261013', 'host': args.host,
              'started_at': now(), 'commit': subprocess.check_output(
                  ['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip(),
              'working_tree_dirty': bool(subprocess.check_output(
                  ['git', 'status', '--porcelain'], cwd=root, text=True)),
              'scope': 'Actual-host hnbctl smoke and distro dependency persistence/restore only',
              'platform_deployed': False, 'steps': []}
    files = [Path(__file__).resolve(), log_path, data_path]

    def execute(label, command, expected=0, timeout=120, input_text=None):
        started = now()
        completed = subprocess.run(command, input=input_text, text=True,
                                   capture_output=True, env=env, timeout=timeout)
        with log_path.open('a') as log:
            log.write('\n## ' + label + ' ' + started + '\n')
            log.write(completed.stdout + completed.stderr)
            log.write('\nexit_code=' + str(completed.returncode) + '\n')
        result['steps'].append({'name': label, 'started_at': started,
                                'finished_at': now(), 'exit_code': completed.returncode})
        print(label + ': exit=' + str(completed.returncode), flush=True)
        if expected is not None and completed.returncode != expected:
            raise RuntimeError(label + ': unexpected exit code; see retained output')
        return completed

    def ssh(label, command, expected=0, timeout=120, input_text=None):
        return execute(label, ['sshpass', '-e', 'ssh', *options, target, command],
                       expected, timeout, input_text)

    def upload(path):
        execute('upload ' + path.name, ['sshpass', '-e', 'scp', *options,
                                      str(path), target + ':' + remote + '/' + path.name])

    try:
        probe = '''import json,os,pathlib,platform,shutil
m={line.split(':')[0]:int(line.split()[1]) for line in pathlib.Path('/proc/meminfo').read_text().splitlines()}
d=shutil.disk_usage('/')
print(json.dumps({'os':pathlib.Path('/etc/os-release').read_text(),'kernel':platform.release(),'arch':platform.machine(),'cpu':os.cpu_count(),'memoryMi':m['MemTotal']//1024,'availableMemoryMi':m['MemAvailable']//1024,'diskBytes':d.total,'freeDiskBytes':d.free,'bootId':pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'tools':{n:shutil.which(n) for n in ['docker','containerd','k3s','kubectl','psql','nats-server']}}))
'''
        initial = ssh('host snapshot before', 'python3 -', input_text=probe)
        result['initial'] = json.loads(initial.stdout)
        if result['initial']['tools']['psql'] or result['initial']['tools']['nats-server']:
            raise RuntimeError('expected fresh dependency host; refuse changing existing services')
        ssh('create isolated working directory', 'test ! -e ' + remote + ' && mkdir -m 700 ' + remote)
        binary = Path(args.binary).resolve()
        result['binary_sha256'] = hashlib.sha256(binary.read_bytes()).hexdigest()
        upload(binary)
        actual = ssh('verify uploaded binary digest', 'sha256sum ' + remote + '/' + binary.name)
        assert actual.stdout.split()[0] == result['binary_sha256']
        cli = remote + '/' + binary.name
        for name in ('version', 'help', 'matrix'):
            ssh('hnbctl ' + name, cli + ' ' + name)
        ssh('hnbctl init draft', cli + ' init --mode bootstrap --profile Minimal --os ubuntu-24.04 -f ' + remote + '/hnb.yaml')
        for name in ('validate', 'preflight', 'install'):
            check = ssh('hnbctl ' + name + ' refuses incomplete draft',
                        cli + ' ' + name + ' -f ' + remote + '/hnb.yaml', expected=None)
            assert check.returncode != 0 and 'INVALID_CONFIG' in check.stderr, check.stderr
        check = ssh('hnbctl verify remains unverified', cli + ' verify --level node --format json', expected=None)
        assert check.returncode != 0 and 'PLATFORM_NOT_VERIFIED' in check.stderr, check.stderr

        # Fixture rendering is explicitly synthetic, never a deployable release.
        sys.path.insert(0, str(root / 'deploy/charts/hnb'))
        from test_profiles import ProfileTests
        rendered = ProfileTests().render(['startup'])
        assert rendered.returncode == 0, rendered.stderr
        manifest = Path(str(prefix) + '-fixture-rendered.yaml')
        manifest.write_text(rendered.stdout)
        files.append(manifest)
        nodes = Path(str(prefix) + '-actual-nodes.yaml')
        import yaml
        declared = {'name': 'cp-1', 'cpuMilli': result['initial']['cpu'] * 1000,
                    'memoryMi': result['initial']['memoryMi'],
                    'diskGi': math.floor(result['initial']['diskBytes'] / (1024 ** 3))}
        nodes.write_text(yaml.safe_dump([declared]))
        files.append(nodes)
        upload(manifest)
        upload(nodes)
        capacity = cli + ' capacity --manifest ' + remote + '/' + manifest.name + ' --nodes ' + remote + '/' + nodes.name + ' --scope startup --cache-source external --backup-source external'
        actual = ssh('hnbctl capacity real host disk rejection', capacity, expected=None)
        report = json.loads(actual.stdout)
        assert actual.returncode != 0 and 'DECLARED_CAPACITY_INSUFFICIENT' in report['reasons']
        assert report['required']['diskGi'] == 27 and not report['installAuthorized']
        result['capacity_actual_host'] = report
        # Explicit hypothetical larger disk control; not an observation.
        hypothetical = Path(str(prefix) + '-hypothetical-40gi-nodes.yaml')
        hypothetical.write_text(yaml.safe_dump([dict(declared, diskGi=40)]))
        files.append(hypothetical)
        upload(hypothetical)
        positive = ssh('hnbctl capacity hypothetical 40Gi positive control',
                       capacity.replace(nodes.name, hypothetical.name))
        result['capacity_hypothetical_40gi'] = json.loads(positive.stdout)

        ssh('install bounded dependency test packages',
            'sudo -n env DEBIAN_FRONTEND=noninteractive apt-get update -qq && sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-16 nats-server', timeout=300)
        source = directory / '20261012-ubuntu-dependency-drill.py'
        drill = Path(str(prefix) + '-dependency.py')
        drill.write_text(source.read_text().replace('20261012', '20261013'))
        files.append(drill)
        result['dependency_source_sha256'] = hashlib.sha256(source.read_bytes()).hexdigest()
        upload(drill)
        run_drill = 'sudo -n python3 ' + remote + '/' + drill.name + ' '
        ssh('dependency seed', run_drill + 'seed')
        ssh('dependency backup', run_drill + 'backup')
        if args.reboot:
            ssh('controlled real reboot', 'sudo -n reboot', expected=None)
            deadline = time.monotonic() + 180
            time.sleep(8)
            while time.monotonic() < deadline:
                try:
                    observed = ssh('reconnect after reboot', 'python3 -', expected=None,
                                   timeout=25, input_text=probe)
                    if observed.returncode == 0:
                        result['after_reboot'] = json.loads(observed.stdout)
                        assert result['initial']['bootId'] != result['after_reboot']['bootId']
                        break
                except subprocess.TimeoutExpired:
                    pass
                time.sleep(5)
            else:
                raise RuntimeError('reboot reconnection deadline exceeded')
        ssh('dependency verify persistence', run_drill + 'verify')
        restored = ssh('dependency restore and verify', run_drill + 'restore')
        result['restore'] = json.loads(restored.stdout.splitlines()[-1])
        denied = ssh('cross-owner database read rejected',
                     "sudo -n -u postgres psql -X -v ON_ERROR_STOP=1 -At -d hnb_drill_market -c 'SET ROLE hnb_drill_platform; SELECT * FROM drill'", expected=None)
        assert denied.returncode != 0 and 'permission denied' in denied.stderr
        ssh('package versions and measured service memory',
            "dpkg-query -W postgresql-16 nats-server; ps -C postgres,nats-server -o pid,comm,rss; free -m; df -h /; sudo -n ss -lntp; sudo -n systemctl is-active postgresql hnb-cloud-drill-nats")
        result['final'] = json.loads(ssh('host snapshot after', 'python3 -', input_text=probe).stdout)
        result['cli_smoke_status'] = 'passed'
        result['dependency_status'] = 'passed'
        result['platform_status'] = 'blocked'
        result['blockers'] = ['Root filesystem smaller than 27Gi Startup planning budget',
                              'No real release lock/digest/identity/TLS/Secret bindings for full platform',
                              'Startup composition not wired to complete hnbctl install acceptance']
        result['exit_code'] = 0
    except Exception as error:
        result['error'] = str(error)
        result['exit_code'] = 1
        raise
    finally:
        result['finished_at'] = now()
        data_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
        digest_path = Path(str(prefix) + '-artifacts.sha256')
        digest_path.write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n'
                                     for p in files if p.exists()))
        print('result: ' + str(data_path), flush=True)


if __name__ == '__main__':
    main()
