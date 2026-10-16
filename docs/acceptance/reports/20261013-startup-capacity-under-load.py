#!/usr/bin/env python3
"""Capacity exercise for the Startup composition on the real host.

Load is generated from the host against the portal Service; per-pod CPU/memory is
read from cgroup v2 (kubepods.slice). This measures behavior under load, it does
not certify a production profile.
"""
import datetime
import json
import statistics
import subprocess
import threading
import time
import urllib.error
import urllib.request

CGROUP_ROOT = "/sys/fs/cgroup/kubepods.slice"
PASSWORD_PATH = "/var/lib/hnb-startup-eval/identity/admin-bootstrap.password"


def kubectl(*args):
    return subprocess.check_output(["sudo", "-n", "k3s", "kubectl", "-n", "hnb-system", *args], text=True)  # noqa: E501


def portal_base():
    ip = kubectl("get", "svc", "portal", "-o", "jsonpath={.spec.clusterIP}" ).strip()
    port = kubectl("get", "svc", "portal", "-o", "jsonpath={.spec.ports[0].port}").strip()
    return f"http://{ip}:{port}"


def pod_map():
    pods = json.loads(kubectl("get", "pods", "-o", "json"))["items"]
    mapping = {}
    for pod in pods:
        name = pod["metadata"]["name"]
        uid = pod["metadata"]["uid"].replace("-", "_")
        slice_dir = f"kubepods-burstable.slice/kubepods-burstable-pod{uid}.slice"
        besteffort = f"kubepods-besteffort.slice/kubepods-besteffort-pod{uid}.slice"
        labels = pod["metadata"].get("labels", {})
        app = labels.get("app.kubernetes.io/name", name)
        containers = []
        for status in pod.get("status", {}).get("containerStatuses", []):
            cid = status.get("containerID", "").replace("containerd://", "")
            containers.append(cid)
        mapping[name] = {"app": app, "slice": slice_dir, "besteffort": besteffort,
                         "containers": containers, "phase": pod.get("status", {}).get("phase")}
    return mapping


def cgroup_path(pod_info, container):
    for prefix in (pod_info["slice"], pod_info["besteffort"]):
        for candidate in (f"cri-containerd-{container}.scope", container):
            path = f"{CGROUP_ROOT}/{prefix}/{candidate}"
            if subprocess.run(["test", "-d", path]).returncode == 0:
                return path
    return None


def read_cpu_mem(pod_info):
    total_cpu_us = 0
    total_mem = 0
    for container in pod_info["containers"]:
        path = cgroup_path(pod_info, container)
        if not path:
            continue
        try:
            cpu = subprocess.check_output(["sudo", "-n", "cat", f"{path}/cpu.stat"], text=True).split()
            usage = int(cpu[cpu.index("usage_usec") + 1])
            mem = int(subprocess.check_output(["sudo", "-n", "cat", f"{path}/memory.current"], text=True))
        except (subprocess.CalledProcessError, FileNotFoundError, ValueError, IndexError):
            continue
        total_cpu_us += usage
        total_mem += mem
    return {"cpu_us": total_cpu_us, "mem": total_mem}


def host_metrics():
    mem = {}
    for line in open("/proc/meminfo"):
        parts = line.split(":")
        if parts[0] in ("MemAvailable", "MemTotal"):
            mem[parts[0]] = int(parts[1].strip().split()[0]) * 1024
    load = open("/proc/loadavg").read().split()[:3]
    return {"mem_available": mem.get("MemAvailable", 0), "mem_total": mem.get("MemTotal", 0),
            "load1": float(load[0]), "load5": float(load[1])}





def main():
    pods = pod_map()
    print("discovered", len(pods), "pods", flush=True)
    password = subprocess.check_output(["sudo", "-n", "cat", PASSWORD_PATH], text=True).strip()
    items = {name: info for name, info in pods.items() if info["phase"] == "Running"}
    PORTAL = portal_base()

    samples = []
    stop_event = threading.Event()
    prev_cpu = {name: read_cpu_mem(info)["cpu_us"] for name, info in items.items()}

    def sampler():
        while not stop_event.is_set():
            row = {"t": time.time(), "host": host_metrics()}
            for name, info in items.items():
                cur = read_cpu_mem(info)
                row[name] = {"cpu_us_delta": cur["cpu_us"] - prev_cpu[name], "mem": cur["mem"]}
                prev_cpu[name] = cur["cpu_us"]
            samples.append(row)
            time.sleep(2)

    threading.Thread(target=sampler, daemon=True).start()
    report = {"started_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "scenarios": {}}

    def login():
        body = ('{"username":"admin","password":"%s"}' % password).encode()
        req = urllib.request.Request(PORTAL + "/api/v1/auth/login", data=body,
                                     headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.loads(resp.read())["data"]["access_token"]

    def authenticated_read(token, path):
        req = urllib.request.Request(PORTAL + path, headers={"Authorization": "Bearer " + token})
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status

    run_state = {"results": [], "lock": threading.Lock()}

    apiserver_ip = kubectl("get", "pod", "-l", "app.kubernetes.io/name=apiserver", "-o",
                           "jsonpath={.items[0].status.podIP}").strip()

    def probe(path):
        try:
            req = urllib.request.Request(f"http://{apiserver_ip}:8080{path}")
            with urllib.request.urlopen(req, timeout=3) as resp:
                return resp.status
        except (urllib.error.URLError, OSError):
            return None

    def pg_connections():
        try:
            out = subprocess.check_output(
                ["sudo", "-n", "k3s", "kubectl", "-n", "hnb-system", "exec", "postgresql-0", "--",
                 "env", "PGUSER=hnb_admin", "PGDATABASE=postgres", "psql", "-tAc",
                 "select count(*) from pg_stat_activity"],
                stderr=subprocess.DEVNULL, text=True, timeout=15)
            return int(out.strip())
        except (subprocess.CalledProcessError, ValueError, subprocess.TimeoutExpired):
            return None

    def worker_loop(_id):
        while time.time() < run_state["stop_at"]:
            try:
                start = time.time()
                token = login()
                run_state["lock"].acquire()
                run_state["results"].append(("login", time.time() - start))
                run_state["lock"].release()
                for path in ("/api/v1/workspaces", "/api/v1/tenants"):
                    start = time.time()
                    authenticated_read(token, path)
                    run_state["lock"].acquire()
                    run_state["results"].append((path, time.time() - start))
                    run_state["lock"].release()
            except urllib.error.HTTPError as exc:
                run_state["lock"].acquire()
                run_state["results"].append(("error", f"HTTP {exc.code}"))
                run_state["lock"].release()
                time.sleep(0.2)
            except (urllib.error.URLError, KeyError, OSError, ValueError) as exc:
                run_state["lock"].acquire()
                run_state["results"].append(("error", type(exc).__name__))
                run_state["lock"].release()
                time.sleep(0.2)

    probes_state = {"live": [], "ready": [], "pg": []}

    def probe_loop(end):
        while time.time() < end:
            probes_state["live"].append(probe("/livez"))
            probes_state["ready"].append(probe("/readyz"))
            conns = pg_connections()
            if conns is not None:
                probes_state["pg"].append(conns)
            time.sleep(2)

    def execute(label, workers, duration):
        run_state["stop_at"] = time.time() + duration
        run_state["results"] = []
        probes_state["live"] = []
        probes_state["ready"] = []
        probes_state["pg"] = []
        threads = [threading.Thread(target=worker_loop, args=(i,)) for i in range(workers)]
        for t in threads:
            t.start()
        probe_thread = threading.Thread(target=probe_loop, args=(time.time() + duration,))
        probe_thread.start()
        time.sleep(duration + 0.5)
        for t in threads:
            t.join()
        probe_thread.join()
        elapsed = duration + 0.5
        latencies = {}
        errors = []
        for row in run_state["results"]:
            if row[0] == "error":
                errors.append(row[1])
                continue
            latencies.setdefault(row[0], []).append(row[1])
        summary = {"workers": workers, "elapsed": elapsed,
                   "errors": len(errors), "error_categories": dict(sorted(__import__("collections").Counter(errors).items())),
                   "ops": sum(len(v) for v in latencies.values())}
        for key, values in latencies.items():
            values.sort()
            total = len(values)
            summary[key] = {
                "count": total,
                "p50_ms": round(statistics.median(values) * 1000, 1),
                "p90_ms": round(values[int(total * 0.9) - 1] * 1000, 1) if total else None,
                "p99_ms": round(values[int(total * 0.99) - 1] * 1000, 1) if total else None,
                "max_ms": round(max(values) * 1000, 1) if total else None,
            }
        if elapsed > 0:
            summary["ops_per_sec"] = round(summary["ops"] / elapsed, 2)
        summary["apiserver_livez"] = __import__("collections").Counter(probes_state["live"])
        summary["apiserver_readyz"] = __import__("collections").Counter(probes_state["ready"])
        pg = probes_state["pg"]
        summary["pg_conns"] = {"samples": len(pg), "min": min(pg) if pg else None,
                               "max": max(pg) if pg else None, "avg": round(statistics.mean(pg), 1) if pg else None}
        report["scenarios"][label] = summary
        print(label, json.dumps(summary), flush=True)

    execute("idle_0w", 0, 15)
    for workers in (1, 2, 4, 8, 16):
        execute(f"load_{workers}w", workers, 30)

    stop_event.set()
    times = samples
    for name, info in items.items():
        deltas = [row.get(name, {}).get("cpu_us_delta", 0) for row in times]
        mems = [row.get(name, {}).get("mem", 0) for row in times]
        entry = {"app": info["app"],
                 "avg_cpu_per_2s_us": round(statistics.mean(deltas), 1),
                 "max_cpu_per_2s_us": max(deltas) if deltas else 0,
                 "cpu_core_equiv_max": round(max(deltas) / 2_000_000, 3) if deltas else 0,
                 "avg_mem_mib": round(statistics.mean(mems) / 1048576, 1) if mems else 0,
                 "max_mem_mib": round(max(mems) / 1048576, 1) if mems else 0}
        print("POD", name, json.dumps(entry), flush=True)
        report.setdefault("pods", {})[name] = entry

    avail = [row["host"]["mem_available"] for row in times]
    report["host"] = {
        "min_mem_available_mib": round(min(avail) / 1048576, 1) if avail else 0,
        "avg_mem_available_mib": round(statistics.mean(avail) / 1048576, 1) if avail else 0,
        "max_load1": max(row["host"]["load1"] for row in times) if times else 0,
        "avg_load1": round(statistics.mean(row["host"]["load1"] for row in times), 2) if times else 0,
        "mem_total_mib": round(max(row["host"]["mem_total"] for row in times) / 1048576, 1) if times else 0,
    }
    report["finished_at"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    out = "/home/ubuntu/capacity-report.json"
    open(out, "w").write(json.dumps(report, indent=2))
    print("REPORT", out, flush=True)
    print(json.dumps(report, indent=2), flush=True)


if __name__ == "__main__":
    main()