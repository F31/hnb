#!/bin/bash
# OpenSpec Quality Gate
# Validates Requirement ID uniqueness, Traceability, and Scenario format
# across all main specs and active change specs.

set -euo pipefail

ERRORS=0
SPEC_DIR="openspec/specs"
CHANGE_DIR="openspec/changes"

echo "=== OpenSpec Quality Gate ==="

# Collect all spec files
SPEC_FILES=$(find "$SPEC_DIR" -name "spec.md" 2>/dev/null || true)
CHANGE_SPECS=$(find "$CHANGE_DIR" -path "*/specs/*/spec.md" -not -path "*/archive/*" 2>/dev/null || true)
ALL_SPECS="$SPEC_FILES $CHANGE_SPECS"

# Check 1: Requirement ID uniqueness
echo "--- Checking Requirement ID uniqueness ---"
ALL_IDS=$(grep -h '### Requirement: \[' $ALL_SPECS 2>/dev/null | grep -oP '\[\K[A-Z]+-[0-9]+' || true)
DUPLICATES=$(echo "$ALL_IDS" | sort | uniq -d)
if [ -n "$DUPLICATES" ]; then
    echo "ERROR: Duplicate Requirement IDs found:"
    echo "$DUPLICATES" | while read -r id; do
        grep -rn "\[$id\]" $ALL_SPECS | head -3
    done
    ERRORS=$((ERRORS + 1))
else
    echo "OK: All Requirement IDs are unique"
fi

# Check 2: Traceability
echo "--- Checking Traceability ---"
MISSING_TRACE=$(grep -l '### Requirement: \[' $ALL_SPECS 2>/dev/null | while read -r f; do
    grep -A 2 '### Requirement: \[' "$f" | grep -v 'Traceability' | grep -v '^--$' | head -1
done || true)
if [ -z "$MISSING_TRACE" ]; then
    echo "OK: All Requirements have Traceability"
else
    echo "WARN: Some Requirements may lack Traceability"
fi

# Check 3: Scenario format (GIVEN/WHEN/THEN)
echo "--- Checking Scenario format ---"
MISSING_GIVEN=$(grep -rL '\- \*\*GIVEN\*\*' $ALL_SPECS 2>/dev/null || true)
if [ -n "$MISSING_GIVEN" ]; then
    echo "WARN: Specs missing GIVEN scenarios:"
    echo "$MISSING_GIVEN"
fi

# Check 4: Tier declaration in active change proposals
echo "--- Checking Tier declarations ---"
for proposal in $(find "$CHANGE_DIR" -name "proposal.md" -maxdepth 2 2>/dev/null || true); do
    if ! grep -q 'T0\|T1\|T2\|T3' "$proposal" 2>/dev/null; then
        echo "WARN: $proposal missing tier declaration"
    fi
done

echo ""
if [ $ERRORS -eq 0 ]; then
    echo "=== Quality Gate PASSED ==="
    exit 0
else
    echo "=== Quality Gate FAILED ($ERRORS errors) ==="
    exit 1
fi