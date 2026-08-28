#!/bin/bash
set -euo pipefail

# E2E Test: RBAC Syncer
# Tests: role assignment → sync → kubectl access → role revocation → permission removal
#
# Usage: ./e2e-test.sh
# Prerequisites: K8s cluster, RBAC Syncer deployed in shadow=false mode

NAMESPACE_PREFIX="hnb-e2e-test"
TENANT_ID="e2e-tenant"
PROJECT_ID="e2e-project"
USER_ID="e2e-user-$(date +%s)"

echo "=== E2E: RBAC Syncer ==="

# Cleanup on exit
cleanup() {
    echo "Cleanup: removing test namespace..."
    kubectl delete namespace "${NAMESPACE_PREFIX}-prod" --ignore-not-found --wait=false
    echo "Cleanup complete."
}
trap cleanup EXIT

# Step 1: Create test namespace
echo "Step 1: Creating test namespace..."
kubectl create namespace "${NAMESPACE_PREFIX}-prod" --dry-run=client -o yaml | kubectl apply -f -
echo "PASS: Namespace created"

# Step 2: Wait for Syncer to backfill (30s poll)
echo "Step 2: Waiting for Syncer backfill..."
sleep 35

# Step 3: Verify operator has NO access initially
echo "Step 3: Verifying no initial access..."
if kubectl auth can-i get pods -n "${NAMESPACE_PREFIX}-prod" --as="hnb:${USER_ID}" 2>/dev/null; then
    echo "FAIL: User should not have access before role assignment"
    exit 1
fi
echo "PASS: No access before role assignment"

# Step 4: Simulate role assignment (insert into user_roles via platform API)
echo "Step 4: Creating role assignment..."
# This would normally go through the platform API
# For e2e, we simulate by calling the platform API
PLATFORM_API="${PLATFORM_API:-http://localhost:8080}"
curl -s -X POST "${PLATFORM_API}/tenants/${TENANT_ID}/users/${USER_ID}:grant" \
    -H "Content-Type: application/json" \
    -d "{\"role\":\"operator\",\"project_id\":\"${PROJECT_ID}\"}" || {
    echo "WARN: Platform API not available, skipping role assignment API test"
    echo "INFO: Manually verify by checking Syncer logs"
}

# Step 5: Wait for Syncer sync (30s poll)
echo "Step 5: Waiting for Syncer to create RoleBinding..."
sleep 35

# Step 6: Verify RoleBinding was created
echo "Step 6: Verifying RoleBinding..."
if kubectl get rolebinding -n "${NAMESPACE_PREFIX}-prod" -l hnb.cloud/tenant-id="${TENANT_ID}" 2>/dev/null | grep -q "hnb"; then
    echo "PASS: RoleBinding created"
else
    echo "FAIL: RoleBinding not found in namespace ${NAMESPACE_PREFIX}-prod"
    echo "Syncer logs:"
    kubectl logs -n hnb-system -l app.kubernetes.io/component=rbac-syncer --tail=20 2>/dev/null || true
    exit 1
fi

# Step 7: Verify kubectl access now works
echo "Step 7: Verifying kubectl access..."
if kubectl auth can-i get pods -n "${NAMESPACE_PREFIX}-prod" --as="hnb:${USER_ID}"; then
    echo "PASS: User can access namespace after role grant"
else
    echo "FAIL: User cannot access namespace after role grant"
    exit 1
fi

# Step 8: Verify operator cannot read secrets
echo "Step 8: Verifying secrets restriction..."
if kubectl auth can-i get secrets -n "${NAMESPACE_PREFIX}-prod" --as="hnb:${USER_ID}"; then
    echo "FAIL: Operator should NOT be able to read secrets"
    exit 1
fi
echo "PASS: Operator cannot access secrets"

# Step 9: Revoke role
echo "Step 9: Revoking role..."
curl -s -X POST "${PLATFORM_API}/tenants/${TENANT_ID}/users/${USER_ID}:revoke" \
    -H "Content-Type: application/json" \
    -d "{\"role\":\"operator\",\"project_id\":\"${PROJECT_ID}\"}" || {
    echo "WARN: Platform API not available, skipping revoke API test"
}

# Step 10: Wait for Syncer to delete RoleBinding
echo "Step 10: Waiting for Syncer to delete RoleBinding..."
sleep 35

# Step 11: Verify RoleBinding was deleted
echo "Step 11: Verifying RoleBinding deletion..."
if kubectl get rolebinding -n "${NAMESPACE_PREFIX}-prod" -l hnb.cloud/tenant-id="${TENANT_ID}" 2>/dev/null | grep -q "hnb"; then
    echo "FAIL: RoleBinding should have been deleted after revocation"
    exit 1
fi
echo "PASS: RoleBinding deleted after revocation"

echo ""
echo "=== E2E Test PASSED ==="
