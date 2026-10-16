#!/usr/bin/env python3
"""Ubuntu 26.04 existing-cluster deployment checks; emits no bearer credentials.

Run as root on the evaluation node. Workload restarts require the explicit flag;
no namespaces, PVCs, data, OS settings or package inventories are removed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def run(*args):
    result = subprocess.run(args, text=True, capture_output=True, timeout=240)
    if result.returncode:
        # Tool stderr can include deployment bindings; retain only command identity.
        raise RuntimeError("command failed: " + args[0] + " (exit " + str(result.returncode) + ")")
    return result.stdout


def kube(*args):
    return run("k3s", "kubectl", *args)


def snapshot(namespace):
    resources = json.loads(kube("-n", namespace, "get", "deployment,statefulset", "-o", "json"))["items"]
    pods = json.loads(kube("-n", namespace, "get", "pods", "-o", "json"))["items"]
    claims = json.loads(kube("-n", namespace, "get", "pvc", "-o", "json"))["items"]
    return {
        "observedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "workloads": [{"kind": r["kind"], "name": r["metadata"]["name"],
                       "desired": r["spec"].get("replicas", 1), "ready": r.get("status", {}).get("readyReplicas", 0),
                       "generation": r["metadata"]["generation"],
                       "observedGeneration": r.get("status", {}).get("observedGeneration", 0)} for r in resources],
        "pods": [{"name": p["metadata"]["name"], "uid": p["metadata"]["uid"],
                  "phase": p["status"].get("phase"), "deleting": bool(p["metadata"].get("deletionTimestamp")),
                  "containers": [{"name": c["name"], "ready": c.get("ready", False),
                                  "restarts": c.get("restartCount", 0), "image": c.get("image"),
                                  "imageID": c.get("imageID")} for c in p["status"].get("containerStatuses", [])]}
                 for p in pods],
        "claims": [{"name": c["metadata"]["name"], "uid": c["metadata"]["uid"],
                    "phase": c["status"].get("phase"), "storageClass": c["spec"].get("storageClassName"),
                    "requested": c["spec"]["resources"]["requests"]["storage"],
                    "capacity": c.get("status", {}).get("capacity", {}).get("storage"),
                    "volumeName": c["spec"].get("volumeName")} for c in claims],
    }


def wait_ready(namespace):
    deadline = time.monotonic() + 180
    while True:
        state = snapshot(namespace)
        workloads = state["workloads"]
        pods = [p for p in state["pods"] if not p["deleting"] and p["phase"] != "Succeeded"]
        if (len(workloads) == len(pods) == 19 and
                all(w["desired"] == w["ready"] == 1 and w["generation"] == w["observedGeneration"] for w in workloads) and
                all(p["containers"] and all(c["ready"] for c in p["containers"]) for p in pods)):
            return state
        if time.monotonic() > deadline:
            raise RuntimeError("full Minimal did not reach exactly 19 current, ready workloads")
        time.sleep(5)


def db_ledgers(namespace):
    return {plane: int(kube("-n", namespace, "exec", "postgresql-0", "--", "psql", "-U", "hnb_admin", "-d", "hnb_" + plane,
                           "-tAc", "SELECT count(*) FROM " + plane + "_schema_migrations").strip())
            for plane in ("platform", "market", "artifact")}


def functions(namespace, identity):
    ip = json.loads(kube("-n", namespace, "get", "svc", "portal", "-o", "json"))["spec"]["clusterIP"]
    base = "http://" + ip
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    def request(path, data=None, token=None):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, headers=headers)
        try:
            with opener.open(req, timeout=20) as response:
                return response.status, json.load(response)
        except urllib.error.HTTPError as error:
            return error.code, {}
    status, payload = request("/api/v1/auth/login", {"username": "admin", "password": (identity / "admin-bootstrap.password").read_text().strip()})
    if status != 200 or "access_token" not in payload.get("data", {}):
        raise RuntimeError("portal login failed")
    token = payload["data"]["access_token"]
    result = {"login": status}
    for path in ("/api/v1/workspaces", "/api/v1/tenants", "/api/v1/agents"):
        status, payload = request(path, token=token)
        result[path] = status
        if status != 200:
            raise RuntimeError("authenticated API failed: " + path)
        if path.endswith("agents"):
            result["agents"] = [{k: agent.get(k) for k in ("cluster_id", "status", "last_heartbeat", "node_count")} for agent in payload]
            if not any(a["status"] == "connected" for a in result["agents"]):
                raise RuntimeError("agent tunnel disconnected")
    status, _ = request("/api/v1/workspaces")
    result["anonymousWorkspaces"] = status
    if status not in (401, 403):
        raise RuntimeError("anonymous caller accessed protected workspaces")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--hnbctl", required=True)
    parser.add_argument("--namespace", default="hnb-system")
    parser.add_argument("--identity-dir", type=Path, default=Path("/var/lib/hnb-minimal-eval/identity"))
    parser.add_argument("--restart-workloads", action="store_true")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    report = {"schemaVersion": "ubuntu-os-deployment-evidence/v1", "runId": "ubuntu2604-adaptation-20261015",
              "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "scope": "existing K3s / amd64 / full Minimal evaluation; not kubeadm bootstrap, offline, HA or 2C4G/15G qualification",
              "assertions": {}, "exitCode": 1}
    try:
        cli = str(Path(args.hnbctl).resolve())
        report["artifactDigests"] = {"hnbctl": "sha256:" + hashlib.sha256(Path(cli).read_bytes()).hexdigest(),
                                     "verifier": "sha256:" + hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
        report["host"] = json.loads(run(cli, "os", "inspect", "--format", "json"))
        assert report["host"]["os"] == "ubuntu-26.04" and not report["host"]["bootstrapCertified"]
        report["assertions"]["actualOSKernelRecognizedWithScopedEvidence"] = True
        report["catalog"] = json.loads(run(cli, "os", "--format=json"))
        report["matrix"] = run(cli, "matrix")
        report["help"] = all("Usage:" in run(cli, *command) for command in (("--help",), ("init", "--help"), ("os", "inspect", "--help")))
        with tempfile.TemporaryDirectory() as temp:
            draft = Path(temp) / "hnb.yaml"
            run(cli, "init", "--mode", "bootstrap", "--os", "ubuntu-26.04", "-f", str(draft))
            report["assertions"]["draftCreatedOwnerOnlyWithoutInstalling"] = draft.stat().st_mode & 0o777 == 0o600 and "user: ubuntu" in draft.read_text()
        report["environment"] = {"k3s": run("k3s", "--version").splitlines()[0],
                                 "apparmor": run("systemctl", "is-active", "apparmor").strip(),
                                 "packages": run("dpkg-query", "-W", "-f=${binary:Package} ${Version}\n", "apt", "ca-certificates", "curl", "gpg", "lvm2", "systemd"),
                                 "bootId": Path("/proc/sys/kernel/random/boot_id").read_text().strip()}
        report["before"] = wait_ready(args.namespace)
        report["ledgersBefore"] = db_ledgers(args.namespace)
        report["functionsBefore"] = functions(args.namespace, args.identity_dir)
        if args.restart_workloads:
            # Database/message broker restarts precede clients; no data is removed.
            for name in ("postgresql", "nats"):
                kube("-n", args.namespace, "rollout", "restart", "statefulset/" + name)
                kube("-n", args.namespace, "rollout", "status", "statefulset/" + name, "--timeout=180s")
            kube("-n", args.namespace, "rollout", "restart", "deployment")
        report["after"] = wait_ready(args.namespace)
        report["ledgersAfter"] = db_ledgers(args.namespace)
        report["functionsAfter"] = functions(args.namespace, args.identity_dir)
        report["assertions"]["nineteenWorkloadsReadyBeforeAndAfter"] = True
        report["assertions"]["claimsUnchangedAcrossRecovery"] = report["before"]["claims"] == report["after"]["claims"]
        report["assertions"]["threePlaneMigrationLedgersPreserved"] = report["ledgersBefore"] == report["ledgersAfter"] and all(report["ledgersAfter"].values())
        report["assertions"]["postgresql2GiNats1GiOnTopoLVM"] = {c["name"]: c["capacity"] for c in report["after"]["claims"]} == {"postgresql-data": "2Gi", "nats-data": "1Gi"} and all(c["storageClass"] == "hnb-topolvm" for c in report["after"]["claims"])
        before = {p["name"]: p["uid"] for p in report["before"]["pods"]}
        report["assertions"]["allWorkloadPodsRecreated"] = args.restart_workloads and all(before.get(p["name"]) != p["uid"] for p in report["after"]["pods"])
        report["assertions"]["loginReadAPIsAgentAndAnonymousRejection"] = True
        report["metrics"] = {"node": kube("top", "node"), "storage": kube("-n", "topolvm-system", "top", "pods")}
        assert all(report["assertions"].values()), "an evidence assertion failed"
        report["exitCode"] = 0
    except Exception as error:
        report["error"] = type(error).__name__ + ": " + str(error)
    finally:
        report["finishedAt"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        args.output.write_text(json.dumps(report, indent=2) + "\n")
        os.chmod(args.output, 0o600)
        print(json.dumps({"exitCode": report["exitCode"], "assertions": report["assertions"], "error": report.get("error")}, indent=2))
    return report["exitCode"]


if __name__ == "__main__":
    raise SystemExit(main())
