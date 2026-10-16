"""Profile rendering + capacity CLI integration; synthetic bindings, not certification."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml
from test_candidate_components import CHART, WholeChartDigestTests


class ProfileTests(unittest.TestCase):
    def render(self, overlays, extra=None):
        values = WholeChartDigestTests().render_values()
        with tempfile.TemporaryDirectory() as tmp:
            binding = Path(tmp) / "bindings.yaml"
            binding.write_text(yaml.safe_dump(values))
            args = ["helm", "template", "fixture", str(CHART), "-f", str(binding)]
            for overlay in overlays:
                args += ["-f", str(CHART / "overlays" / (overlay + ".yaml"))]
            args += extra or []
            return subprocess.run(args, capture_output=True, text=True, timeout=120)

    def test_minimal_includes_t1_and_excludes_optional_packs(self):
        result = self.render(["minimal"])
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = list(yaml.safe_load_all(result.stdout))
        workloads = {d["metadata"]["name"] for d in docs if d and d["kind"] in ("Deployment", "StatefulSet")}
        for name in ("alert-manager", "cluster-agent", "plugin-worker", "portal", "network-registry", "cilium-provider"):
            self.assertIn(name, workloads)
        for name in ("edge-provider", "karmada-provider", "gslb-controller", "calico-provider", "kube-ovn-provider", "network-provider"):
            self.assertNotIn(name, workloads)
        for d in docs:
            if d and d["kind"] in ("Deployment", "StatefulSet"):
                self.assertEqual(d["spec"]["replicas"], 1)
                for c in d["spec"]["template"]["spec"]["containers"]:
                    self.assertIn("resources", c, d["metadata"]["name"])
                    self.assertIn("cpu", c["resources"]["requests"])
                    self.assertIn("memory", c["resources"]["requests"])

    def test_chart_rejects_missing_required_and_optional_packs(self):
        for override, reason in (("edge-provider.enabled=true", "MINIMAL_OPTIONAL_PACK_ENABLED"),
                                 ("alert-manager.enabled=false", "MINIMAL_REQUIRED_COMPONENT_MISSING")):
            result = self.render(["minimal"], ["--set", override])
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(reason, result.stderr)

    def test_ha_profiles_render_in_cluster_backings(self):
        for overlays in (["minimal", "lite-ha"], ["minimal", "lite-ha", "standard-ha"],
                         ["minimal", "lite-ha", "standard-ha", "enterprise"]):
            with self.subTest(overlays=overlays):
                result = self.render(overlays)
                self.assertEqual(result.returncode, 0, result.stderr)
                docs = list(yaml.safe_load_all(result.stdout))
                cluster = next(d for d in docs if d and d.get("kind") == "Cluster"
                               and d["metadata"]["name"] == "postgresql")
                self.assertGreaterEqual(cluster["spec"]["instances"], 3)
                self.assertIn("@sha256:", cluster["spec"]["imageName"])
                nats = next(d for d in docs if d and d.get("kind") == "StatefulSet"
                            and d["metadata"]["name"] == "nats")
                self.assertGreaterEqual(nats["spec"]["replicas"], 3)
                self.assertEqual(nats["spec"]["serviceName"], "nats-headless")
                self.assertFalse(any(d and d.get("kind") == "StatefulSet"
                                     and d["metadata"]["name"] == "postgresql" for d in docs))

    def test_in_cluster_ha_requires_three_control_domains(self):
        result = self.render(["minimal", "lite-ha"], ["--set", "global.controlPlaneDomains=1"])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("HA_CONTROL_NODES_INSUFFICIENT", result.stderr)

    def test_ha_external_backing_disables_bundled_data_plane(self):
        result = self.render(["minimal", "lite-ha"], [
            "--set", "global.backings=external",
            "--set", "postgresql.enabled=false",
            "--set", "nats.enabled=false",
        ])
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = list(yaml.safe_load_all(result.stdout))
        self.assertFalse(any(d and d.get("kind") == "Cluster" for d in docs))
        mismatched = self.render(["minimal", "lite-ha"], ["--set", "global.backings=external"])
        self.assertNotEqual(mismatched.returncode, 0)
        self.assertIn("EXTERNAL_BACKING_MISCONFIGURED", mismatched.stderr)

    def test_startup_loads_only_core_and_adds_capabilities_on_demand(self):
        core = {"apiserver", "platform-api", "operation-worker", "extension-controller",
                "app-market", "cluster-router", "tunnel-server", "portal", "postgresql", "nats"}
        capabilities = {
            "capability-runtime": {"runtime-target-lifecycle-provider", "kubernetes-provider"},
            "capability-network": {"network-registry", "cilium-provider", "gateway-provider"},
            "capability-agent": {"cluster-agent", "rbac-syncer"},
            "capability-alerts": {"alert-manager"},
            "capability-plugins": {"plugin-worker"},
        }
        for overlays, expected in [(["startup"], core)] + [
                (["startup", pack], core | components) for pack, components in capabilities.items()]:
            with self.subTest(overlays=overlays):
                result = self.render(overlays)
                self.assertEqual(result.returncode, 0, result.stderr)
                docs = list(yaml.safe_load_all(result.stdout))
                workloads = {d["metadata"]["name"] for d in docs if d and d["kind"] in ("Deployment", "StatefulSet")}
                self.assertEqual(workloads, expected)
        full = self.render(["startup", *capabilities, "minimal"])
        self.assertEqual(full.returncode, 0, full.stderr)

    def test_startup_rejects_broken_core_and_network_dependencies(self):
        for override, reason in (("platform-api.enabled=false", "STARTUP_REQUIRED_COMPONENT_MISSING"),
                                 ("cilium-provider.enabled=true", "COMPONENT_DEPENDENCY_MISSING"),
                                 ("global.deploymentProfile=Minimal", "STARTUP_NOT_CERTIFIED_PROFILE"),
                                 ("global.deploymentStage=unknown", "UNKNOWN_DEPLOYMENT_STAGE")):
            result = self.render(["startup"], ["--set", override])
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(reason, result.stderr)

    def test_operator_resource_override_reaches_rendered_container(self):
        result = self.render(["startup"], ["--set", "apiserver.resources.requests.cpu=175m"])
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = next(d for d in yaml.safe_load_all(result.stdout)
                          if d and d["kind"] == "Deployment" and d["metadata"]["name"] == "apiserver")
        self.assertEqual(deployment["spec"]["template"]["spec"]["containers"][0]["resources"]["requests"]["cpu"], "175m")

    def test_startup_preserves_bff_service_boundary(self):
        result = self.render(["startup"])
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = list(yaml.safe_load_all(result.stdout))
        for name in ("apiserver", "platform-api", "app-market"):
            service = next(d for d in docs if d and d["kind"] == "Service" and d["metadata"]["name"] == name)
            deployment = next(d for d in docs if d and d["kind"] == "Deployment" and d["metadata"]["name"] == name)
            self.assertEqual(service["spec"]["type"], "ClusterIP")
            for key, value in service["spec"]["selector"].items():
                self.assertEqual(deployment["spec"]["template"]["metadata"]["labels"][key], value)
            self.assertIn(service["spec"]["ports"][0]["targetPort"],
                          {p["name"] for p in deployment["spec"]["template"]["spec"]["containers"][0]["ports"]})
        bff = next(d for d in docs if d and d["kind"] == "Deployment" and d["metadata"]["name"] == "apiserver")
        env = {e["name"]: e for e in bff["spec"]["template"]["spec"]["containers"][0]["env"]}
        self.assertEqual(env["PLATFORM_API_URL"]["value"], "http://platform-api:8080")
        self.assertEqual(env["APP_MARKET_URL"]["value"], "http://app-market:8080")
        self.assertEqual(env["REQUIRE_PLATFORM_API"]["value"], "true")

    def test_startup_capacity_on_two_core_four_gib_node(self):
        result = self.render(["startup"])
        self.assertEqual(result.returncode, 0, result.stderr)
        with tempfile.TemporaryDirectory() as tmp:
            manifest, nodes = Path(tmp) / "render.yaml", Path(tmp) / "nodes.yaml"
            manifest.write_text(result.stdout)
            nodes.write_text(yaml.safe_dump([{"name": "cp-1", "cpuMilli": 2000, "memoryMi": 4096, "diskGi": 40}]))
            command = ["go", "run", ".", "capacity", "--manifest", str(manifest), "--nodes", str(nodes),
                       "--scope", "startup", "--cache-source", "external", "--backup-source", "external"]
            run = subprocess.run(command, cwd=CHART.parents[2] / "cmd" / "hnbctl",
                                 capture_output=True, text=True, timeout=120)
            self.assertEqual(run.returncode, 0, run.stderr + run.stdout)
            report = json.loads(run.stdout)
            self.assertTrue(report["fitsDeclaredCapacity"])
            self.assertFalse(report["installAuthorized"])
            self.assertEqual(len(report["workloads"]), 10)
            self.assertEqual(report["requests"]["cpuMilli"], 1075)
            self.assertEqual(report["requests"]["memoryMi"], 1792)
            self.assertEqual(report["requests"]["diskGi"], 15)
            self.assertLessEqual(report["required"]["cpuMilli"], 2000)
            self.assertLessEqual(report["required"]["memoryMi"], 4096)
            self.assertLessEqual(report["required"]["diskGi"], 40)
            local = subprocess.run(command[:-4], cwd=CHART.parents[2] / "cmd" / "hnbctl",
                                   capture_output=True, text=True, timeout=120)
            self.assertNotEqual(local.returncode, 0)
            self.assertIn("DECLARED_CAPACITY_INSUFFICIENT", json.loads(local.stdout)["reasons"])

    def test_small_disk_startup_budget_requires_explicit_reserve_and_placement(self):
        result = self.render(["startup", "startup-small-disk"])
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = list(yaml.safe_load_all(result.stdout))
        pvcs = {d["metadata"]["name"]: d["spec"]["resources"]["requests"]["storage"]
                for d in docs if d and d["kind"] == "PersistentVolumeClaim"}
        self.assertEqual(sorted(pvcs.values()), ["1Gi", "2Gi"])
        self.assertEqual(sum(d is not None and d["kind"] in ("Deployment", "StatefulSet") for d in docs), 10)
        with tempfile.TemporaryDirectory() as tmp:
            manifest, nodes = Path(tmp) / "render.yaml", Path(tmp) / "nodes.yaml"
            manifest.write_text(result.stdout)
            node = {"name": "cp-1", "cpuMilli": 2000, "memoryMi": 3913, "diskGi": 10, "reserveDiskGi": 6}
            nodes.write_text(yaml.safe_dump([node]))
            command = ["go", "run", ".", "capacity", "--manifest", str(manifest), "--nodes", str(nodes),
                       "--scope", "startup", "--cache-source", "external", "--backup-source", "external"]
            cwd = CHART.parents[2] / "cmd" / "hnbctl"
            run = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=120)
            self.assertEqual(run.returncode, 0, run.stderr + run.stdout)
            report = json.loads(run.stdout)
            self.assertEqual(report["requests"], {"cpuMilli": 1075, "memoryMi": 1792, "diskGi": 3})
            self.assertEqual(report["required"]["diskGi"], 10)
            self.assertEqual(report["systemReserve"]["diskGi"], 6)
            self.assertFalse(report["installAuthorized"])
            # Local cache/backup remain budgeted; small PVCs do not erase them.
            local = subprocess.run(command[:-4], cwd=cwd, capture_output=True, text=True, timeout=120)
            self.assertNotEqual(local.returncode, 0)
            self.assertEqual(json.loads(local.stdout)["required"]["diskGi"], 35)
            # Keeping the default 10Gi system reserve must still reject a 10Gi host.
            node.pop("reserveDiskGi")
            nodes.write_text(yaml.safe_dump([node]))
            blocked = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=120)
            self.assertNotEqual(blocked.returncode, 0)
            self.assertEqual(json.loads(blocked.stdout)["required"]["diskGi"], 14)
            self.assertIn("DECLARED_CAPACITY_INSUFFICIENT", json.loads(blocked.stdout)["reasons"])

    def test_rendered_resources_feed_capacity_cli(self):
        result = self.render(["minimal"])
        self.assertEqual(result.returncode, 0, result.stderr)
        root = CHART.parents[2]
        with tempfile.TemporaryDirectory() as tmp:
            manifest = Path(tmp) / "render.yaml"
            manifest.write_text(result.stdout)
            nodes = Path(tmp) / "nodes.yaml"
            nodes.write_text(yaml.safe_dump([{"name": "cp-1", "controlPlane": True, "failureDomain": "zone-a",
                                             "cpuMilli": 16000, "memoryMi": 32768, "diskGi": 200}]))
            run = subprocess.run(["go", "run", ".", "capacity", "--manifest", str(manifest), "--nodes", str(nodes)],
                                 cwd=root / "cmd" / "hnbctl", capture_output=True, text=True, timeout=120)
            self.assertEqual(run.returncode, 0, run.stderr)
            report = json.loads(run.stdout)
            self.assertFalse(report["installAuthorized"])
            self.assertTrue(report["fitsDeclaredCapacity"])
            self.assertEqual(report["requests"]["diskGi"], 3)
            self.assertEqual(report["requests"]["cpuMilli"], 1250)
            self.assertEqual(report["requests"]["memoryMi"], 2032)
            nodes.write_text(yaml.safe_dump([{"name": "cp-1", "cpuMilli": 2000, "memoryMi": 4096, "diskGi": 40}]))
            blocked = subprocess.run(["go", "run", ".", "capacity", "--manifest", str(manifest), "--nodes", str(nodes)],
                                     cwd=root / "cmd" / "hnbctl", capture_output=True, text=True, timeout=120)
            self.assertNotEqual(blocked.returncode, 0)
            self.assertIn("DECLARED_CAPACITY_INSUFFICIENT", json.loads(blocked.stdout)["reasons"])

    def test_full_minimal_small_install_preserves_workloads_and_burst_limits(self):
        result = self.render(["minimal"])
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        workloads = [d for d in docs if d["kind"] in ("Deployment", "StatefulSet")]
        self.assertEqual(len(workloads), 19)
        for workload in workloads:
            name = workload["metadata"]["name"]
            defaults = yaml.safe_load((CHART / "charts" / name / "values.yaml").read_text())
            for container in workload["spec"]["template"]["spec"]["containers"]:
                self.assertEqual(container["resources"]["limits"], defaults["resources"]["limits"], name)
        with tempfile.TemporaryDirectory() as tmp:
            manifest, nodes = Path(tmp) / "render.yaml", Path(tmp) / "nodes.yaml"
            manifest.write_text(result.stdout)
            nodes.write_text(yaml.safe_dump([{"name": "cp-1", "cpuMilli": 2000,
                                              "memoryMi": 3913, "diskGi": 15}]))
            command = ["go", "run", ".", "capacity", "--manifest", str(manifest), "--nodes", str(nodes),
                       "--scope", "startup", "--cache-source", "external", "--backup-source", "external"]
            cwd = CHART.parents[2] / "cmd" / "hnbctl"
            run = subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=120)
            self.assertEqual(run.returncode, 0, run.stderr + run.stdout)
            report = json.loads(run.stdout)
            self.assertEqual(report["required"], {"cpuMilli": 1875, "memoryMi": 3260, "diskGi": 14})
            self.assertFalse(report["installAuthorized"])
            local = subprocess.run(command[:-4], cwd=cwd, capture_output=True, text=True, timeout=120)
            self.assertNotEqual(local.returncode, 0)
            self.assertIn("DECLARED_CAPACITY_INSUFFICIENT", json.loads(local.stdout)["reasons"])


if __name__ == "__main__":
    unittest.main()
