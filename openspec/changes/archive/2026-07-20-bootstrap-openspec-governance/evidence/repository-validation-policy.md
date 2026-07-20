# Repository hosting and validation policy

## Decision

Decision date: 2026-07-20.

GitHub is used only for Git repository hosting. The project does not use GitHub Actions or another continuous integration service. OpenSpec and Contracts quality gates remain mandatory before a change enters review or is archived, and their command, fixed tool versions, associated commit SHA, and results are stored as versioned evidence.

The authoritative commands are:

```bash
node scripts/validate-openspec.mjs
node scripts/validate-contracts.mjs
node --test scripts/validate-openspec.test.mjs scripts/contracts.test.mjs
```

Future CI adoption requires a separate OpenSpec change. Any adapter must call these repository commands rather than duplicate validation logic.

## GitHub Actions removal rehearsal

Two GitHub workflows were removed after the repository-hosting decision:

- `.github/workflows/openspec-quality-gate.yml`
- `.github/workflows/contracts-quality-gate.yml`

Queued workflow runs were cancelled. The temporary repository-scoped self-hosted runner was stopped, unregistered, and removed from the test host. GitHub subsequently reported zero registered repository runners. Removing these adapters did not modify any OpenSpec source, Contracts source, generated output, validation script, or automated test.

The test host continues to run its pre-existing Nova, PostgreSQL, Redis, Nginx, Docker, and containerd services. Environment and backup files discovered under `/opt` were tightened from mode `0644` to `0600`; that independent security improvement was retained.

## Enforcement and limitation

The repository has no remote required status checks. Enforcement currently depends on change authors and reviewers running the authoritative commands and rejecting missing or failing evidence. This limitation is explicit and accepted; a task cannot be marked complete and a change cannot be archived without local gate evidence.
