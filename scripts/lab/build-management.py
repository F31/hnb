#!/usr/bin/env python3
"""Build digest-pinned development images from this checkout, off the small hosts."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
SERVICES = ("apiserver", "platform-api", "operation-worker", "extension-controller", "app-market",
            "cluster-router", "tunnel-server", "kubernetes-provider", "runtime-target-lifecycle-provider", "cluster-agent")


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    out = args.output_dir.resolve()
    out.mkdir(mode=0o700, parents=True, exist_ok=True)
    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64")
    manifest = {"schemaVersion": "hnb-lab-build/v1", "scope": "startup plus runtime development candidate; not a formal release",
                "commit": run("git", "rev-parse", "HEAD"), "images": {}}
    for service in SERVICES:
        context = out / service
        context.mkdir(exist_ok=True)
        binary = context / "service"
        subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(binary), "./cmd/" + service], cwd=ROOT, env=env, check=True)
        ref = "hnb-lab/" + service + ":20261018"
        subprocess.run(["docker", "build", "--provenance=false", "-q", "-t", ref,
                        "-f", str(ROOT / "deploy/lab/Dockerfile.service"), str(context)], check=True)
        descriptor = json.loads(run("docker", "image", "inspect", ref))[0]["Descriptor"]
        manifest["images"][service] = {"reference": ref, "repository": "docker.io/hnb-lab/" + service,
                                        "digest": descriptor["digest"], "binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest()}
    for name, source in (("hnbctl", "./cmd/hnbctl"), ("migrate", "scripts/lab/migrate.go")):
        subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(out / name), source], cwd=ROOT, env=env, check=True)
    subprocess.run(["pnpm", "-C", str(ROOT / "web"), "build:all"], check=True)
    base = json.loads(run("docker", "image", "inspect", "hnb-eval/portal:20261013"))[0]
    manifest["portalRuntimeBase"] = base["Descriptor"]["digest"]
    subprocess.run(["docker", "build", "--provenance=false", "-q", "-t", "hnb-lab/portal:20261018", "-f",
                    str(ROOT / "deploy/lab/Dockerfile.portal"), str(ROOT / "web")], check=True)
    for name, ref, repository in (("portal", "hnb-lab/portal:20261018", "docker.io/hnb-lab/portal"),
                                  ("postgresql", "postgres:16", "docker.io/library/postgres"),
                                  ("nats", "nats:2.11.1-scratch", "docker.io/library/nats")):
        manifest["images"][name] = {"reference": ref, "repository": repository,
                                      "digest": json.loads(run("docker", "image", "inspect", ref))[0]["Descriptor"]["digest"]}
    (out / "build-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("Built", len(manifest["images"]), "digest-pinned images; manifest:", out / "build-manifest.json")


if __name__ == "__main__":
    main()
