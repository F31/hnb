# Infrastructure verifier first failure

- run_id: `s2-s3-lab-20261018-first-failure`
- commit: `533d8ae41b386d9ae738528f73641720d40f997f` plus working-tree lab scripts
- started_at: `2026-10-18T00:11:50Z`
- finished_at: `2026-10-18T00:12:07Z`
- command: `python3 scripts/lab/verify-environment.py --output docs/acceptance/reports/20261018-s2-s3-lab-infrastructure.json`
- exit_code: 1
- failure: `CERTIFICATE_VERIFY_FAILED: self-signed certificate`

The first verifier connected to the public IP with IP SNI. Traefik selected its
default certificate rather than the certificate associated with `app.hnb.test`.
The independent clusters, CSI and persistent marker had already been observed;
the verifier failed before final assertions were calculated. The earlier curl
probe with `--resolve app.hnb.test:443:106.75.15.35 --cacert ...` succeeded.

Correction: connect to the declared IP but send/verify `app.hnb.test` SNI and the
same HTTP Host. Keep normal hostname/certificate verification enabled; do not use
`insecureSkipVerify` or `curl -k`. The retained successful JSON is a separate run.
