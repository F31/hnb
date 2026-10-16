"""Evaluation-only lost-request injection; uses real Portal admission and provider.

Run as root on the prepared K3s evaluation host. --refresh-token only stages a
fresh, 60-second test credential; normal mode creates one new test Operation,
injects loss with the worker stopped, then observes recovery. No success facts
are written to the database. JSON progress survives a lost SSH connection.
"""
import argparse
import base64
import json
import os
import subprocess
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path

import yaml
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils

ROOT = Path('/home/ubuntu')
RESULT = ROOT / 'redrive-v4-recovery.json'
PROVIDER = 'runtime-target.lifecycle.kubernetes'
AUD = 'hnb-runtime-target-lifecycle-provider'
TOKEN_KEY = 'runtime-target-lifecycle-provider.token'


def k(*args, stdin=None):
    return subprocess.check_output(['k3s', 'kubectl', '-n', 'hnb-system', *args], input=stdin,
                                   text=True, timeout=200)


def q(sql):
    return k('exec', 'postgresql-0', '--', 'env', 'PGUSER=hnb_admin', 'PGDATABASE=hnb_platform',
             'psql', '-v', 'ON_ERROR_STOP=1', '-tAc', sql).strip()


def svc(name):
    return json.loads(k('get', 'svc', name, '-o', 'json'))['spec']['clusterIP']


def http(path, payload, token=None, headers=None):
    head = {'Content-Type': 'application/json', **(headers or {})}
    if token:
        head['Authorization'] = 'Bearer ' + token
    request = urllib.request.Request('http://' + svc('portal') + path,
                                     data=json.dumps(payload).encode(), headers=head)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=25) as response:
            return response.status, json.loads(response.read())
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read())


def secret(name):
    return {key: base64.b64decode(value) for key, value in
            json.loads(k('get', 'secret', name, '-o', 'json'))['data'].items()}


def b64(value):
    return base64.urlsafe_b64encode(value).rstrip(b'=').decode()


def refresh_token():
    manifest = json.loads(secret('hnb-eval-identity-public')['manifest.json'])
    signing = serialization.load_pem_private_key(secret('hnb-eval-identity-private')['private.pem'], password=None)
    tenant = q('select distinct tenant_id from operations limit 1')
    now = int(time.time())
    claims = {'profileVersion': 'hnb.identity/v1', 'iss': manifest['issuer'], 'aud': [AUD],
              'sub': 'operation-worker', 'type': 'service', 'tenantId': tenant,
              'membershipId': 'runtime-eval-member', 'tenantMembershipIds': ['runtime-eval-member'],
              'policyVersion': 'runtime-eval:1', 'allowedActions': ['execute'],
              'scopedPermissions': [{'resourceKind': 'runtimeProvider', 'resourceId': PROVIDER,
                                     'action': 'execute', 'tenantId': tenant}],
              'iat': now, 'nbf': now, 'exp': now + 60, 'auth_time': now,
              'jti': str(uuid.uuid4()), 'keyId': manifest['activeKeyId'], 'algorithm': 'ES256'}
    content = b64(json.dumps({'alg': 'ES256', 'typ': 'at+jwt', 'kid': manifest['activeKeyId']},
                             separators=(',', ':')).encode()) + '.' + b64(json.dumps(claims, separators=(',', ':')).encode())
    r, s = utils.decode_dss_signature(signing.sign(content.encode(), ec.ECDSA(hashes.SHA256())))
    token = content + '.' + b64(r.to_bytes(32, 'big') + s.to_bytes(32, 'big'))
    current = json.loads(k('get', 'secret', 'hnb-eval-service-tokens', '-o', 'json'))
    current['data'][TOKEN_KEY] = base64.b64encode(token.encode()).decode()
    k('apply', '-f', '-', stdin=json.dumps(current))


def stamp():
    return datetime.now(timezone.utc).isoformat()


def refresh_binding(token):
    service = json.loads(k('get', 'svc', 'platform-api', '-o', 'json'))['spec']
    request = urllib.request.Request('http://' + service['clusterIP'] + ':' + str(service['ports'][0]['port'])
                                     + '/v1/providers/' + PROVIDER + '/manifest',
                                     headers={'Authorization': 'Bearer ' + token})
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=20) as response:
        digest = response.headers['X-Provider-Manifest-Digest']
        manifest = json.loads(response.read())
    assert digest.startswith('sha256:') and len(digest) == 71
    assert manifest['conformance_level'] == 'production_ready'
    assert datetime.fromisoformat(manifest['conformance_expires_at'].replace('Z', '+00:00')) > datetime.now(timezone.utc)
    deployment = json.loads(k('get', 'deployment', 'operation-worker', '-o', 'json'))
    container = next(c for c in deployment['spec']['template']['spec']['containers'] if c['name'] == 'operation-worker')
    binding = json.loads(next(e['value'] for e in container['env'] if e['name'] == 'RUNTIME_PROVIDERS'))
    if binding[PROVIDER]['providerDigest'] != digest or binding[PROVIDER]['providerVersion'] != manifest['version']:
        binding[PROVIDER].update(providerDigest=digest, providerVersion=manifest['version'])
        patch = {'spec': {'template': {'spec': {'containers': [{'name': 'operation-worker', 'env': [
            {'name': 'RUNTIME_PROVIDERS', 'value': json.dumps(binding)}]}]}}}}
        k('patch', 'deployment', 'operation-worker', '--type=strategic', '-p', json.dumps(patch))
    return digest


def snapshot(operation):
    return json.loads(q("select json_build_object('status', o.status, 'completed_steps', o.completed_steps,"
                        " 'total_steps', o.total_steps, 'steps', (select json_agg(json_build_object("
                        " 'id', s.id, 'status', s.status, 'retry_count', s.retry_count, 'error', s.error_message,"
                        " 'started_at', s.started_at, 'completed_at', s.completed_at)) from operation_steps s"
                        " where s.operation_id=o.id), 'requests', (select json_agg(json_build_object("
                        " 'message_id', e.message_id, 'status', e.status, 'attempt', e.attempt,"
                        " 'max_attempts', e.max_attempts, 'key_matches_step', e.idempotency_key=s.idempotency_key))"
                        " from outbox_events e join operation_steps s on s.id=e.step_id"
                        " where e.operation_id=o.id and e.message_type='hnb.command.operation.step-requested.v1'))"
                        " from operations o where o.id='%s'" % operation))


def save(result):
    RESULT.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--refresh-token', action='store_true')
    parser.add_argument('--refresh-binding', action='store_true')
    args = parser.parse_args()
    if args.refresh_token:
        refresh_token()
        print('test service token staged; lifetime remains 60 seconds')
        return
    os.umask(0o077)
    run_id = 'redrive-v4-' + uuid.uuid4().hex[:8]
    result = {'run_id': run_id, 'started_at': stamp(), 'scope': 'lost request then real provider execution'}
    admin_pw = Path('/var/lib/hnb-startup-eval/identity/admin-bootstrap.password').read_text().strip()
    status, body = http('/api/v1/auth/login', {'username': 'admin', 'password': admin_pw})
    assert status == 200, f'login status={status}'
    token = body['data']['access_token']
    result['provider_manifest_digest'] = refresh_binding(token)
    if args.refresh_binding:
        print(json.dumps({'provider_manifest_digest': result['provider_manifest_digest']}))
        return
    save(result)
    kube = yaml.safe_load(Path('/etc/rancher/k3s/k3s.yaml').read_text())
    kube['clusters'][0]['cluster']['server'] = 'https://kubernetes.default.svc'
    kube['users'] = [{'name': 'default', 'user': {'token': k('create', 'token', 'default', '--duration=2h').strip()}}]
    status, _ = http('/api/v1/secrets:register', {'purpose': 'kubeconfig', 'scope': 'runtime-eval',
                     'name': run_id, 'value': base64.b64encode(yaml.safe_dump(kube).encode()).decode()}, token)
    result['credential_status'] = status
    assert status == 201, f'credential registration status={status}'
    try:
        k('scale', 'deployment/operation-worker', '--replicas=0')
        # Wait for actual termination, not a fixed delay, before injecting loss.
        k('wait', '--for=delete', 'pod', '-l', 'app.kubernetes.io/name=operation-worker', '--timeout=90s')
        # Sign in again after termination; admin access tokens also last 60 seconds.
        status, body = http('/api/v1/auth/login', {'username': 'admin', 'password': admin_pw})
        assert status == 200
        token = body['data']['access_token']
        correlation = str(uuid.uuid4())
        intent = {'apiVersion': 'hnb.io/v1', 'kind': 'ImportRuntimeTarget',
                  'metadata': {'idempotencyKey': run_id, 'correlationId': correlation},
                  'spec': {'targetKind': 'KubernetesTarget', 'displayName': run_id,
                           'credentialSecretRef': {'provider': 'local-aes', 'scope': 'runtime-eval',
                                                   'name': run_id, 'version': '1'}}}
        status, body = http('/api/v1/runtime-intents', intent, token,
                            {'Idempotency-Key': run_id, 'X-Correlation-ID': correlation})
        result['intent_status'] = status
        if status != 202:
            result['admission_error'] = body
            raise RuntimeError(f'intent rejected: {status}')
        operation = (body.get('data') or body)['operationId']
        uuid.UUID(operation)
        result['operation_id'] = operation
        result['injection'] = q("update outbox_events set status='published', published_at=now()"
                                " where operation_id='%s' and message_type='hnb.command.operation.step-requested.v1'"
                                " and status='pending'" % operation)
        assert result['injection'] == 'UPDATE 1', result['injection']
        result['before'] = snapshot(operation)
        save(result)
        # Default idle threshold is 120s. Mint only after that window, immediately
        # before startup, so the initial sweep can use a still-valid test token.
        time.sleep(125)
        refresh_token()
        result['token_minted_at'] = stamp()
        k('scale', 'deployment/operation-worker', '--replicas=1')
        k('rollout', 'status', 'deployment/operation-worker', '--timeout=180s')
        result['worker_restored_at'] = stamp()
        save(result)
        deadline = time.monotonic() + 150
        while time.monotonic() < deadline:
            result['after'] = snapshot(operation)
            if result['after']['status'] in ('succeeded', 'failed', 'cancelled'):
                break
            time.sleep(3)
        assert result['after']['status'] == 'succeeded', result['after']
        requests = result['after']['requests']
        assert len(requests) == 1 and requests[0]['attempt'] >= 1 and requests[0]['key_matches_step']
        result['passed'] = True
    except Exception as error:
        result['passed'] = False
        result['error'] = str(error)
        raise
    finally:
        k('scale', 'deployment/operation-worker', '--replicas=1')
        result['finished_at'] = stamp()
        save(result)


if __name__ == '__main__':
    main()
