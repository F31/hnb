#!/usr/bin/env python3
"""Read-only verification of the declared three-host infrastructure lab.

Use SSH keys, or provide SSHPASS for sshpass at runtime. This is not an HNB
import/Operation, application lifecycle, HA or production acceptance verifier.
"""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import ssl
import socket
import subprocess
import time
import urllib.request


ROOT = Path(__file__).resolve().parents[2]
HOSTS = {"management": "106.75.37.46", "target": "106.75.15.35", "worker": "106.75.25.31"}
MARKER = "hnb-persist-after-recreation-20261018"


def ssh(host, command):
    args = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "ubuntu@" + host, command]
    if os.environ.get("SSHPASS"):
        args = ["sshpass", "-e"] + [a.replace("BatchMode=yes", "BatchMode=no") for a in args]
    result = subprocess.run(args, capture_output=True, text=True, timeout=45)
    if result.returncode:
        raise RuntimeError("remote observation failed on " + host)
    return result.stdout


def kube(host, args):
    result = json.loads(ssh(host, "sudo -n k3s kubectl " + args + " -o json"))
    for item in result.get("items", [result]):
        item.get("metadata", {}).pop("managedFields", None)
        item.get("metadata", {}).get("annotations", {}).pop("kubectl.kubernetes.io/last-applied-configuration", None)
    return result


def pod_view(pod):
    return {"name": pod["metadata"]["name"], "uid": pod["metadata"]["uid"],
            "node": pod["spec"]["nodeName"], "phase": pod["status"]["phase"],
            "containers": [{k: c.get(k) for k in ("name", "ready", "restartCount", "image", "imageID")}
                           for c in pod["status"].get("containerStatuses", [])]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    report = {
        "schemaVersion": "s2-s3-lab-infrastructure/v1", "runId": "s2-s3-lab-20261018",
        "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "scope": "independent cluster, CSI and Gateway infrastructure only; no HNB import/application/HA acceptance",
        "commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
        "command": "python3 scripts/lab/verify-environment.py --output " + str(args.output),
        "assertions": {}, "exitCode": 1,
    }
    try:
        report["hosts"] = {}
        observe = (
            "python3 -c 'import json,os,platform; from pathlib import Path; "
            "s=os.statvfs(\"/\"); print(json.dumps(dict(hostname=platform.node(),kernel=platform.release(),"
            "availableBytes=s.f_bavail*s.f_frsize,bootID=Path(\"/proc/sys/kernel/random/boot_id\").read_text().strip())))'"
        )
        for role, host in HOSTS.items():
            report["hosts"][role] = {"sshHost": host, **json.loads(ssh(host, observe))}
        report["clusters"] = {}
        for role in ("management", "target"):
            host = HOSTS[role]
            nodes = kube(host, "get nodes")["items"]
            report["clusters"][role] = {
                "uid": kube(host, "get namespace kube-system")["metadata"]["uid"],
                "nodes": [{"name": n["metadata"]["name"], "uid": n["metadata"]["uid"],
                           "version": n["status"]["nodeInfo"]["kubeletVersion"],
                           "ready": any(c["type"] == "Ready" and c["status"] == "True" for c in n["status"]["conditions"])} for n in nodes],
                "storageClass": kube(host, "get storageclass hnb-topolvm"),
                "csiDriver": kube(host, "get csidriver topolvm.io")["metadata"]["name"],
                "storagePods": [pod_view(p) for p in kube(host, "get pods -n topolvm-system")["items"]],
                "metrics": ssh(host, "sudo -n k3s kubectl top nodes"),
            }
        target = HOSTS["target"]
        report["gateway"] = kube(target, "get gateway hnb-lab -n hnb-gateway")
        report["route"] = kube(target, "get httproute probe -n hnb-lab-probes")
        report["claim"] = kube(target, "get pvc probe-data -n hnb-lab-probes")
        report["probePod"] = pod_view(kube(target, "get pod probe-server -n hnb-lab-probes"))
        report["marker"] = ssh(target, "sudo -n k3s kubectl exec -n hnb-lab-probes probe-server -- cat /data/index.html").strip()
        ca = ssh(target, "sudo -n cat /var/lib/hnb-lab/tls/probe.crt")
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        report["externalProbes"] = {}
        request = urllib.request.Request("http://" + target + "/", headers={"Host": "app.hnb.test"})
        with opener.open(request, timeout=20) as response:
            report["externalProbes"]["http"] = {"status": response.status, "body": response.read(4096).decode().strip()}
        # Connect to the declared IP, but validate and send the route's real SNI.
        with socket.create_connection((target, 443), timeout=20) as tcp:
            with ssl.create_default_context(cadata=ca).wrap_socket(tcp, server_hostname="app.hnb.test") as tls:
                tls.sendall(b"GET / HTTP/1.1\r\nHost: app.hnb.test\r\nConnection: close\r\n\r\n")
                response = http.client.HTTPResponse(tls)
                response.begin()
                report["externalProbes"]["https"] = {"status": response.status, "body": response.read(4096).decode().strip()}
        clusters = report["clusters"]
        assertions = report["assertions"]
        assertions["independentClusterIdentities"] = clusters["management"]["uid"] != clusters["target"]["uid"]
        assertions["declaredNodesReady"] = (
            {n["name"] for n in clusters["management"]["nodes"]} == {"hnb-mgmt"} and
            {n["name"] for n in clusters["target"]["nodes"]} == {"hnb-target-control", "hnb-target-worker"} and
            all(n["ready"] and n["version"] == "v1.36.5+k3s1" for c in clusters.values() for n in c["nodes"])
        )
        assertions["csiReadyBothClusters"] = all(
            c["storageClass"]["provisioner"] == "topolvm.io" and c["storageClass"]["reclaimPolicy"] == "Retain" and
            len(c["storagePods"]) == expected and all(
                p["phase"] == "Running" and p["containers"] and
                all(s["ready"] for s in p["containers"]) for p in c["storagePods"]
            ) for c, expected in ((clusters["management"], 2), (clusters["target"], 3))
        )
        assertions["gatewayProgrammed"] = any(c["type"] == "Programmed" and c["status"] == "True" for c in report["gateway"]["status"]["conditions"])
        assertions["routeAcceptedAndResolved"] = any(
            all(any(c["type"] == kind and c["status"] == "True" for c in p["conditions"]) for kind in ("Accepted", "ResolvedRefs"))
            for p in report["route"].get("status", {}).get("parents", [])
        )
        assertions["csiVolumeBoundAndMarkerPreserved"] = report["claim"]["status"]["phase"] == "Bound" and report["marker"] == MARKER
        assertions["externalHTTPAndVerifiedTLS"] = all(p["status"] == 200 and p["body"] == MARKER for p in report["externalProbes"].values())
        assertions["minimumSevenGiBFreePerHost"] = all(h["availableBytes"] >= 7 * 1024**3 for h in report["hosts"].values())
        report["artifactDigests"] = {str(p): "sha256:" + hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in map(Path, (
            "scripts/lab/bootstrap-k3s.sh", "scripts/lab/prepare-loop-storage.sh", "scripts/lab/verify-environment.py",
            "deploy/lab/target-probes.yaml", "deploy/lab/traefik-values.yaml", "deploy/lab/probe-route.yaml",
            "deploy/storage/topolvm/values-minimal.yaml", "deploy/storage/topolvm/values-s2-s3-lab.yaml",
        ))}
        if not all(assertions.values()):
            raise RuntimeError("an infrastructure assertion failed")
        report["exitCode"] = 0
    except Exception as error:
        report["error"] = type(error).__name__ + ": " + str(error)
    report["finishedAt"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: report[k] for k in ("exitCode", "assertions")}))
    return report["exitCode"]


if __name__ == "__main__":
    raise SystemExit(main())
