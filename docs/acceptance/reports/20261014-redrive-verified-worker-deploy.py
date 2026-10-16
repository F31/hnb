"""Import and deploy an evaluation worker, verifying content rather than alias names.

Run on the K3s evaluation host as root with --archive, --manifest and --source.
The manifest must contain one digest-pinned operation-worker Deployment.
"""
import argparse
import hashlib
import json
import subprocess

import yaml


def run(*args):
    return subprocess.check_output(args, text=True, timeout=240)


def verify_content(ctr, digest):
    content = subprocess.check_output([*ctr, "content", "get", digest], timeout=30)
    actual = "sha256:" + hashlib.sha256(content).hexdigest()
    if actual != digest:
        raise RuntimeError(f"content hash mismatch: expected {digest}, got {actual}")
    return json.loads(content)


def verify_target(ctr, source, expected):
    rows = [line.split() for line in run(*ctr, "images", "ls").splitlines()[1:]]
    targets = [row[2] for row in rows if row[0] == source]
    if len(targets) != 1:
        raise RuntimeError(f"image reference missing or ambiguous: {source}")
    target = targets[0]
    manifest = verify_content(ctr, target)
    if target != expected:
        matches = [entry for entry in manifest.get("manifests", [])
                   if entry.get("platform", {}).get("os") == "linux"
                   and entry.get("platform", {}).get("architecture") == "amd64"
                   and entry["digest"] == expected]
        if len(matches) != 1:
            raise RuntimeError(f"{source} resolves to {target}, not expected amd64 manifest {expected}")
        manifest = verify_content(ctr, expected)
    if "config" not in manifest or "layers" not in manifest:
        raise RuntimeError("expected an image manifest")
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--source", required=True, help="canonical imported tag, including docker.io/")
    parser.add_argument("--namespace", default="hnb-system")
    args = parser.parse_args()
    with open(args.manifest) as stream:
        deployment = yaml.safe_load(stream)
    namespace = deployment["metadata"].get("namespace", args.namespace)
    name = deployment["metadata"]["name"]
    container = next(c for c in deployment["spec"]["template"]["spec"]["containers"]
                     if c["name"] == "operation-worker")
    image = container["image"]
    repo, expected = image.split("@sha256:")
    expected = "sha256:" + expected
    ctr = ["k3s", "ctr", "-n", "k8s.io"]
    print(run(*ctr, "images", "import", "--platform", "linux/amd64", args.archive), flush=True)
    # Validate the source BEFORE it is allowed to replace a digest-named alias.
    target = verify_target(ctr, args.source, expected)
    canonical = image if repo.startswith("docker.io/") else "docker.io/" + image
    print(run(*ctr, "images", "tag", "--force", args.source, canonical), flush=True)
    verify_target(ctr, canonical, expected)
    kubectl = ["k3s", "kubectl", "-n", namespace]
    print(run(*kubectl, "apply", "-f", args.manifest), flush=True)
    # Restart even when repairing the content behind a previously incorrect alias.
    print(run(*kubectl, "rollout", "restart", "deployment/" + name), flush=True)
    print(run(*kubectl, "rollout", "status", "deployment/" + name, "--timeout=180s"), flush=True)
    pods = json.loads(run(*kubectl, "get", "pods", "-l", "app.kubernetes.io/name=" + name, "-o", "json"))
    active = [p for p in pods["items"] if not p["metadata"].get("deletionTimestamp")]
    if not active:
        raise RuntimeError("no active worker pods")
    for pod in active:
        status = next(s for s in pod["status"]["containerStatuses"] if s["name"] == "operation-worker")
        if not status["ready"] or not status["imageID"].endswith("@" + expected):
            raise RuntimeError(f"runtime image mismatch: {status['imageID']}, expected {expected}")
    print(json.dumps({"requested_manifest": expected, "imported_target": target,
                      "runtime_image_ids": [next(s["imageID"] for s in p["status"]["containerStatuses"]
                                                  if s["name"] == "operation-worker") for p in active]}, indent=2))


if __name__ == "__main__":
    main()
