# Runbook: NATS JetStream Operations

## Installation

### Minimal (Single Node)

```bash
kubectl apply -f deploy/nats/minimal/nats-deployment.yaml
```

### Lite HA (3 Nodes)

```bash
kubectl apply -f deploy/nats/lite-ha/nats-deployment.yaml
```

### Verify Installation

```bash
kubectl get pods -n hnb-messaging
kubectl get svc -n hnb-messaging
nats -s nats://nats.hnb-messaging:4222 stream list
```

## Certificate Rotation

### Procedure

1. Generate new TLS certificates (CA, server, client)
2. Update Secret: `kubectl create secret tls nats-tls -n hnb-messaging --cert=new.crt --key=new.key --dry-run=client -o yaml | kubectl apply -f -`
3. Reload NATS config: `kubectl exec -n hnb-messaging nats-jetstream-0 -- nats-server --signal reload`
4. Verify: `nats -s nats://nats.hnb-messaging:4222 --tlscert=new.crt --tlskey=new.key pub test.tls "ok"`
5. Rotate service credentials

## Capacity Expansion

### Storage

```bash
# Scale PVC (StatefulSet)
kubectl patch pvc data-nats-jetstream-0 -n hnb-messaging -p '{"spec":{"resources":{"requests":{"storage":"100Gi"}}}}'
```

### Replicas (Lite HA)

```bash
# Scale from 3 to 5 nodes
kubectl scale statefulset nats-jetstream -n hnb-messaging --replicas=5
# Update route list in ConfigMap
kubectl edit configmap nats-config -n hnb-messaging
# Rolling restart
kubectl rollout restart statefulset nats-jetstream -n hnb-messaging
```

## Backlog Handling

### Monitoring

```bash
# Check consumer lag
nats -s nats://nats.hnb-messaging:4222 consumer info commands operation-worker

# Check pending events
kubectl exec -n hnb-messaging nats-jetstream-0 -- nats stream info commands
```

### Clearing Backlog

1. Identify slow consumer
2. Scale consumer instances: `kubectl scale deployment <consumer> --replicas=3`
3. If backlog persists, check for poison messages
4. Move poison messages to failed subject
5. Increase MaxAckPending temporarily if needed

## Failed Subject Recovery

### List Failed Messages

```bash
nats -s nats://nats.hnb-messaging:4222 consumer info failed-messages <consumer>
```

### Redrive Message

```bash
curl -X POST https://api.hnb.cloud/api/v1/admin/messaging/redrive \
  -H "Authorization: Bearer <token>" \
  -d '{"messageId":"<uuid>","targetSubject":"<subject>","reason":"Issue resolved"}'
```

## Backup and Restore

### JetStream Backup

```bash
# Snapshot JetStream data directory
kubectl exec -n hnb-messaging nats-jetstream-0 -- tar czf /tmp/jetstream-backup.tar.gz /data/jetstream
kubectl cp hnb-messaging/nats-jetstream-0:/tmp/jetstream-backup.tar.gz ./jetstream-backup.tar.gz
```

### JetStream Restore

```bash
kubectl cp ./jetstream-backup.tar.gz hnb-messaging/nats-jetstream-0:/tmp/
kubectl exec -n hnb-messaging nats-jetstream-0 -- tar xzf /tmp/jetstream-backup.tar.gz -C /data/
kubectl delete pod nats-jetstream-0 -n hnb-messaging
```

## Disaster Recovery

### RPO/RTO

| Tier | RPO | RTO |
|------|-----|-----|
| Minimal | 10 min | 30 min |
| Lite HA | 0 (no message loss) | 5 min |
| Standard HA | 0 | 2 min |
| Enterprise | 0 | < 1 min |

### Recovery Procedure

1. Restore PostgreSQL Operation Store first (authoritative)
2. Restore NATS JetStream from backup
3. Verify Outbox events match between stores
4. Resume Outbox Relay
5. Monitor consumer lag and backlog

## Uninstall

### Minimal

```bash
kubectl delete -f deploy/nats/minimal/nats-deployment.yaml
```

### Lite HA

```bash
kubectl delete -f deploy/nats/lite-ha/nats-deployment.yaml
```