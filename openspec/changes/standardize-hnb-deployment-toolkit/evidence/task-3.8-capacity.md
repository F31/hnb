# 3.8 Four profiles and capacity planning

- task: 3.8
- requirements: DBND-004, PINST-002
- date: 2026-10-12
- status: done (planning implementation; HA delivery still blocked)

## Implementation

`deploy/charts/hnb/overlays/` contains Minimal/LiteHA/StandardHA/Enterprise
values and documented merge order, backing/storage/system/cache/backup/failure
domain budgets. T1 alert-manager and cluster-agent remain enabled in Minimal;
Edge/Karmada/GSLB and alternate network Packs remain disabled independently of
deployment size. HA uses external backings because bundled PG/NATS are single
instances, and rendering refuses uncertified HA without a bypass switch.

`hnbctl capacity --profile Minimal --manifest rendered.yaml --nodes nodes.yaml`
uses rendered requests (sidecar sum, init max, replicas), PVC/claim-template
storage, per-node reserve, cache, full-copy backup and 30% headroom. It checks
aggregate and single-Pod capacity, rejects missing requests and unsupported
models, preserves unknown external PVC capacity and HA qualification failures.
Reports include the actual manifest digest and never include environment/Secret
values. Inputs are declared planning capacity; installAuthorized is always false.

Rendering exposed a misplaced lifecycle-provider resources block under its
Secret volume. It now belongs to the container; missing requests cannot silently
understate capacity.

## Actual test outcomes

Commands executed in the workspace:

```text
cd cmd/hnbctl
go test ./... -count=1             PASS (CLI and toolkit)
go vet ./...                      PASS
go test -race ./... -count=1       PASS (CLI and toolkit)

python3 deploy/charts/hnb/test_candidate_components.py  13 tests PASS
python3 deploy/charts/hnb/test_profiles.py               8 tests PASS
openspec validate --all --strict                        29 passed, 0 failed
node scripts/validate-openspec.mjs                       31 specs / 242 requirements / 377 scenarios
git diff --check                                        PASS
```

Profile suite includes actual Helm rendering into a temporary manifest and
actual capacity CLI execution. Resource calibration is applied to parent and
subchart defaults. Earlier claims that parent/CLI overrides could not reach
subcharts were incorrect: the synthetic bindings fixture overrode resources.
The fixture now binds artifacts/credentials without replacing resource policy,
and an explicit CLI override regression verifies Helm precedence.
Calibrated Minimal requests are **1725m CPU, 2624 MiB RAM, 30 GiB PVC**;
core services run at 125m/256Mi, standard services at 100m/128Mi, low-frequency controllers at
50m/64Mi.
The capacity model now accepts a **per-node system reserve override**,
**external download-cache/backup placement** (`--cache-source`, `--backup-source`),
and a **planning scope** (`--scope delivery|startup`): startup uses 10% headroom
and a lean default reserve (500m/1024Mi/10Gi) for a dev/test starting tier; it is
explicitly not a production certification.

Planning floors (external cache + backup, startup scope, lean reserve):
- required = 2398m CPU / 3911 MiB RAM / 43 GiB disk;
- a declared **3 vCPU / 6 GiB / 50 GiB** node passes planning (2C4G does not).
- A 2 vCPU / 4 GiB host still fails even with external placement and a lean
  reserve: CPU is short by 398m and a 40Gi disk by 3Gi. Full 19-workload Minimal
  does not fit 2C4G/40Gi. Process consolidation does not automatically reduce
  summed requests; runtime savings require measurement, not assumed subtraction.

16 CPU/32 GiB/200 GiB declared input passes planning; 2 CPU/4 GiB/40 GiB
fails. All three HA overlay chains fail HA_PROFILE_UNCERTIFIED. Unit tests prove
three domains alone still cannot qualify HA, and aggregate capacity cannot hide
a Pod that fits no node.

Synthetic image digests and Secret names are render fixtures only. No cluster
installation, load test, storage recovery, failure drill or release certification
is claimed. Tasks 5.x/7.4/7.5 retain their independent acceptance requirements.

## 2026-10-13: explicit lightweight initial composition

`startup.yaml` retains Core, Portal and Market as 8 separate application
components plus persistent PostgreSQL/NATS. Five capability overlays reuse the
existing independent implementations, enabling runtime/network/Agent/alerts/plugin
execution on demand without compiling Providers into Core or merging plane DBs.
Startup cannot declare itself a complete Minimal/HA profile; complete Minimal
still requires all existing components. This is deployment-time opt-in loading,
not an automatic plugin installer or scale-to-zero controller.

Actual rendering: **10 workloads, 1075m CPU, 1792Mi RAM, 15Gi PVC**. Fresh-install
PVC defaults are 10Gi PG + 5Gi NATS; existing PVCs must not be shrunk. Startup
scope with external cache/backup requires **1683m / 2996Mi / 27Gi**, and the
actual capacity CLI accepts declared 2C4Gi/40Gi with installAuthorized=false.
Local cache/backup instead requires 65Gi and correctly rejects the same node.

Single-Pod admission now uses the same selected/overridden reserve as aggregate
planning; negative or capacity-exhausting reserves are rejected. Previously
single-Pod checks hard-coded the delivery reserve even in startup scope.

Usage and limitations: `docs/deployment/lightweight-startup.md`. Startup does not
claim a successful full-platform installation, load test, login or actual runtime
Operation; hnbctl's complete delivery acceptance remains independent.

Internal ClusterIP Services for apiserver/platform-api/app-market and explicit
PLATFORM_API_URL/APP_MARKET_URL/REQUIRE_PLATFORM_API bindings preserve the BFF
service boundary in the initial composition. Render tests verify selectors,
named ports and service addresses.

Final checks executed for this optimization:

```text
cmd/hnbctl: go build ./...                              PASS
cmd/hnbctl: go test -race ./... -count=1                 PASS
cmd/hnbctl: go vet ./...                                PASS
Chart: python3 -m unittest test_profiles test_candidate_components
                                                       26 tests PASS
openspec validate --all --strict                        29 passed
node scripts/validate-openspec.mjs                      31 specs / 242 requirements / 377 scenarios
git diff --check                                       PASS
```

### Opt-in small-disk evaluation

`startup-small-disk.yaml` overrides only fresh-install PVC requests to PG 2Gi /
NATS 1Gi. Startup defaults remain unchanged. With explicit node reserveDiskGi=6,
startup scope and external cache/backup assumptions, required disk is
ceil(3 × 1.1) + 6 = **10Gi**. With the default reserve it is 14Gi; with local
cache/backup and the 6Gi reserve it is 35Gi. The node input floor now accepts
exactly 10Gi; input acceptance alone still does not authorize installation.

Chart/CLI integration tests verify all three placement/reserve outcomes and
preserve workload count, CPU/memory and installAuthorized=false. The updated CLI
was uploaded to 106.75.34.133, digest-checked, and accepted the small-data
external-placement planning on its actual 18Gi root filesystem, while rejecting
the local-placement control. See `docs/acceptance/reports/20261013-ubuntu-small-disk-planning.md`.
The 6Gi system/runtime/local-image budget is declared, not measured certification;
no real 10Gi full-platform deployment is claimed. Updated Chart suite: 27 tests PASS.
