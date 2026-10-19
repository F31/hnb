"""Render-only candidate checks; synthetic digests are not release evidence.

Run: python3 deploy/charts/hnb/test_candidate_components.py
Requires Helm and PyYAML. No cluster access or resource writes.
"""
from pathlib import Path
import subprocess
import unittest

import yaml


CHART = Path(__file__).resolve().parent
DIGEST = "sha256:" + "a" * 64  # Test fixture only.


def command(component, overrides=None, action="template"):
    values = {
        "image.digest": DIGEST,
        "global.imagePullPolicy": "IfNotPresent",
        "global.nats.url": "tls://nats:4222",
    }
    if component != "portal":
        values.update({"db.existingSecret": "fixture-db", "masterKeySecret": "fixture-kms"})
    if component == "runtime-target-lifecycle-provider":
        values["identity.existingManifestSecret"] = "fixture-manifest"
    if component == "plugin-worker":
        values["nats.existingClientTLSSecret"] = "fixture-nats-tls"
    values.update(overrides or {})
    args = ["helm", action]
    if action == "template":
        args.append("fixture")
    args.append(str(CHART / "charts" / component))
    for key, value in values.items():
        args += ["--set" if key.endswith(".enabled") else "--set-string", f"{key}={value}"]
    return subprocess.run(args, capture_output=True, text=True, timeout=30)


class WholeChartDigestTests(unittest.TestCase):
    """Every component image must be digest-pinned; no tag may render."""

    ALL_CHARTS = sorted(p.parent.name for p in (CHART / "charts").glob("*/Chart.yaml"))

    def render_values(self):
        """Build the parent-chart values that satisfy every component's inputs."""
        doc = {"global": {"imageRegistry": "", "imagePullPolicy": "IfNotPresent"}}
        for name in WholeChartDigestTests.ALL_CHARTS:
            raw = yaml.safe_load((CHART / "charts" / name / "values.yaml").read_text()) or {}
            # Shipped defaults first, then the fixture values that satisfy each
            # component's declared required inputs.
            entry = dict(raw)
            # Bind artifacts/credentials without overriding the resource policy
            # shipped by the parent chart. Helm merges subchart defaults itself.
            entry.pop("resources", None)
            entry["enabled"] = True
            entry["image"] = dict(raw.get("image") or {}, digest=DIGEST)
            if name == "kubernetes-provider":
                entry["allowedNamespaces"] = ["fixture-runtime"]
            if name == "operation-worker":
                entry["serviceTokenFiles"] = ["fixture-provider.token"]
                entry["sandbox"] = {**raw.get("sandbox", {}), "digest": DIGEST}
            if name == "apiserver":
                # The signing-key init container image is pinned like every other.
                entry["sandbox"] = {**raw.get("sandbox", {}), "digest": DIGEST}
            if name == "postgresql":
                # Bundled infrastructure always requires an operator-provided
                # admin Secret; there is no default credential.
                entry["adminSecret"] = "fixture-admin"
                entry["storage"] = {**raw.get("storage", {}), "className": "fixture-sc"}
                entry["planes"] = {p: {"database": "hnb_" + p, "role": "hnb_" + p, "existingSecret": "fixture-"+p+"-pg"}
                                   for p in ("platform", "market", "artifact")}
            if name == "nats":
                entry["storage"] = {**raw.get("storage", {}), "className": "fixture-sc"}
                entry["planes"] = {p: {"existingSecret":"fixture-"+p+"-nats"} for p in ("platform", "market", "artifact")}
                entry["tls"] = {"existingSecret":"fixture-nats-tls"}
            for key, value in {
                "nats": {"url": "tls://nats:4222", "existingClientTLSSecret": "fixture-tls",
                         "existingAccountSecret": "fixture-nats-account"},
                "db": {"existingSecret": "fixture-db", "dsn": "postgres://fixture/fixture"},
                "masterKeySecret": "fixture-kms",
                "identity": {"existingManifestSecret": "fixture-manifest",
                             "issuer": "https://issuer.hnb.test",
                             "existingPrivateKeySecret": "fixture-key"},
                "platform": {"namespace": "hnb-system"},
                "staleChallenge": {"existingSecret": "fixture-challenge"},
                "serviceTokenSecret": "fixture-token",
                "runtimeProviders": "http://provider:18082",
                "podCIDR": "10.244.0.0/16", "serviceCIDR": "10.96.0.0/12",
                "apiBaseURL": "http://apiserver:8080",
                "tunnel": {"apiBaseURL": "http://apiserver:8080"},
                "gateway": {"apiBaseURL": "http://apiserver:8080"},
                "networkPlugin": "cilium",
                "registry": {"url": "http://127.0.0.1:80"},
                "cluster": {"podCIDR": "10.244.0.0/16", "serviceCIDR": "10.96.0.0/12"},
            }.items():
                if key not in raw:
                    continue
                if isinstance(raw[key], dict) and isinstance(value, dict):
                    entry[key] = {**raw[key], **{k: v for k, v in value.items() if k in raw[key]}}
                else:
                    entry[key] = value
            doc[name] = entry
        return doc

    def render_all(self, mutate=None):
        doc = WholeChartDigestTests().render_values()
        if mutate:
            mutate(doc)
        path = CHART / "test-whole-values.yaml"
        path.write_text(yaml.safe_dump(doc, sort_keys=True))
        try:
            result = subprocess.run(["helm", "template", "fixture", str(CHART), "-f", str(path)],
                                    capture_output=True, text=True, timeout=120)
        finally:
            path.unlink(missing_ok=True)
        return result

    def test_every_component_image_is_digest_pinned(self):
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        images = sorted({line.split("image:", 1)[1].strip().strip('"')
                         for line in result.stdout.splitlines() if line.strip().startswith("image:")})
        self.assertTrue(images, "no images rendered")
        for image in images:
            self.assertIn("@sha256:", image, f"tag-pinned image rendered: {image}")
            digest = image.split("@", 1)[1]
            self.assertRegex(digest, r"^sha256:[0-9a-f]{64}$")
            self.assertNotIn("latest", image)

    def test_platform_api_requires_master_key_secret(self):
        result = self.render_all(lambda doc: doc["platform-api"].update(masterKeySecret=""))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("platform-api.masterKeySecret is required", result.stderr)

    def test_platform_api_master_key_uses_secret_reference(self):
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = next(d for d in yaml.safe_load_all(result.stdout)
                          if d and d["kind"] == "Deployment" and
                          d["metadata"]["name"] == "platform-api")
        env = {e["name"]: e for e in deployment["spec"]["template"]["spec"]["containers"][0]["env"]}
        self.assertEqual(env["HNB_MASTER_KEY"], {
            "name": "HNB_MASTER_KEY",
            "valueFrom": {"secretKeyRef": {"name": "fixture-kms", "key": "key"}},
        })

    def test_operation_worker_stages_owner_only_provider_tokens(self):
        """iam.FileTokenSource rejects group-readable files, and a Secret mount
        under fsGroup resolves to 0440, so the worker needs a staged copy."""
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = next(d for d in yaml.safe_load_all(result.stdout)
                          if d and d["kind"] == "Deployment" and
                          d["metadata"]["name"] == "operation-worker")
        pod = deployment["spec"]["template"]["spec"]
        self.assertEqual(pod["securityContext"]["runAsUser"], 65532)
        init = next(c for c in pod["initContainers"] if c["name"] == "stage-provider-tokens")
        self.assertIn("@sha256:", init["image"])
        self.assertIn("chmod 0400", " ".join(init["command"]))
        self.assertIn("fixture-provider.token", " ".join(init["command"]))
        self.assertEqual(init["securityContext"]["runAsUser"], 0)
        container = pod["containers"][0]
        staged = next(m for m in container["volumeMounts"] if m["name"] == "staged-provider-tokens")
        self.assertEqual(staged["mountPath"], "/var/run/hnb-provider-tokens")
        self.assertTrue(staged["readOnly"])
        volume = next(v for v in pod["volumes"] if v["name"] == "staged-provider-tokens")
        self.assertEqual(volume["emptyDir"], {"medium": "Memory", "sizeLimit": "64Ki"})

    def test_operation_worker_requires_explicit_provider_token_staging(self):
        result = self.render_all(lambda doc: doc["operation-worker"].update(serviceTokenFiles=[]))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("serviceTokenFiles", result.stderr)

    def test_operation_worker_renewal_adapter_receives_identity_configuration(self):
        def configure(doc):
            doc["operation-worker"]["serviceIdentity"] = {
                "enabled": True, "issuerURL": "http://apiserver:8080",
                "issuer": "https://issuer.hnb.test/", "existingManifestSecret": "renewal-public-keys",
            }

        result = self.render_all(configure)
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = next(d for d in yaml.safe_load_all(result.stdout)
                          if d and d["kind"] == "Deployment" and d["metadata"]["name"] == "operation-worker")
        pod = deployment["spec"]["template"]["spec"]
        env = {e["name"]: e.get("value") for e in pod["containers"][0]["env"]}
        self.assertEqual(env["SERVICE_IDENTITY_ISSUER_URL"], "http://apiserver:8080")
        self.assertEqual(env["SERVICE_IDENTITY_ISSUER"], "https://issuer.hnb.test/")
        self.assertEqual(env["SERVICE_IDENTITY_KEY_MANIFEST_FILE"], "/var/run/hnb-service-identity/manifest.json")
        self.assertEqual(env["SERVICE_IDENTITY_KEY_RELOAD_INTERVAL"], "5s")
        mount = next(m for m in pod["containers"][0]["volumeMounts"] if m["name"] == "service-identity-keys")
        self.assertTrue(mount["readOnly"])
        secret = next(v["secret"] for v in pod["volumes"] if v["name"] == "service-identity-keys")
        self.assertEqual(secret, {"secretName": "renewal-public-keys", "defaultMode": 0o440})

    def test_operation_worker_renewal_requires_issuer_url_and_public_keys(self):
        for missing in ("issuerURL", "issuer", "existingManifestSecret"):
            with self.subTest(missing=missing):
                def configure(doc):
                    doc["operation-worker"]["serviceIdentity"] = {
                        "enabled": True, "issuerURL": "http://apiserver:8080",
                        "issuer": "https://issuer.hnb.test/", "existingManifestSecret": "renewal-public-keys",
                        missing: "",
                    }

                result = self.render_all(configure)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("serviceIdentity." + missing + " is required", result.stderr)

    def test_kubernetes_provider_requires_explicit_namespaces(self):
        result = self.render_all(lambda doc: doc["kubernetes-provider"].update(allowedNamespaces=[]))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("kubernetes-provider.allowedNamespaces is required", result.stderr)

    def test_kubernetes_provider_port_scope_and_readiness_match(self):
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        deployment = next(d for d in yaml.safe_load_all(result.stdout)
                          if d and d["kind"] == "Deployment" and
                          d["metadata"]["name"] == "kubernetes-provider")
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        env = {e["name"]: e.get("value") for e in container["env"]}
        self.assertEqual(env["ALLOWED_NAMESPACES"], "fixture-runtime")
        self.assertEqual(env["LISTEN_ADDRESS"], ":8080")
        self.assertEqual(env["MAX_REPLICAS"], "10")
        self.assertEqual(container["ports"][0]["containerPort"], 8080)
        self.assertEqual(container["readinessProbe"]["httpGet"], {"path": "/healthz", "port": "http"})

    def test_components_authenticate_nats_plane_account_from_files(self):
        """The server authenticates planes by username/password, so a client
        certificate alone cannot connect. Every NATS client must mount its plane
        account Secret and read the credential from files, never from values."""
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        checked = 0
        for doc in docs:
            if doc["kind"] not in ("Deployment", "StatefulSet"):
                continue
            container = doc["spec"]["template"]["spec"]["containers"][0]
            env = {e["name"]: e for e in container.get("env", [])}
            if "NATS_URL" not in env:
                continue
            checked += 1
            self.assertEqual(env["NATS_USER_FILE"]["value"], "/var/run/nats-account/user",
                             doc["metadata"]["name"])
            self.assertEqual(env["NATS_PASSWORD_FILE"]["value"], "/var/run/nats-account/password",
                             doc["metadata"]["name"])
            mounts = {m["name"]: m for m in container.get("volumeMounts", [])}
            self.assertEqual(mounts["nats-account"]["mountPath"], "/var/run/nats-account",
                             doc["metadata"]["name"])
            self.assertTrue(mounts["nats-account"]["readOnly"], doc["metadata"]["name"])
            volumes = {v["name"]: v for v in doc["spec"]["template"]["spec"].get("volumes", [])}
            secret = volumes["nats-account"]["secret"]
            self.assertEqual(secret["secretName"], "fixture-nats-account", doc["metadata"]["name"])
            # Readable by the pod's fsGroup (the non-root owner), never by others.
            self.assertEqual(secret["defaultMode"], 0o440, doc["metadata"]["name"])
            self.assertEqual([i["key"] for i in secret["items"]], ["user", "password"],
                             doc["metadata"]["name"])
            # The credential must never be rendered inline.
            self.assertNotIn("password\"", yaml.safe_dump(env))
        self.assertGreaterEqual(checked, 6)

    def test_nats_account_credentials_are_optional_when_unset(self):
        def drop(doc):
            for name, entry in doc.items():
                if isinstance(entry, dict) and isinstance(entry.get("nats"), dict):
                    entry["nats"].pop("existingAccountSecret", None)

        result = self.render_all(drop)
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        for doc in docs:
            if doc["kind"] not in ("Deployment", "StatefulSet"):
                continue
            container = doc["spec"]["template"]["spec"]["containers"][0]
            env = {e["name"] for e in container.get("env", [])}
            if "NATS_URL" not in env:
                continue
            self.assertNotIn("NATS_USER_FILE", env, doc["metadata"]["name"])
            self.assertNotIn("nats-account",
                             {m["name"] for m in (container.get("volumeMounts") or [])},
                             doc["metadata"]["name"])

    def test_database_secret_precedes_inline_dsn_for_every_client(self):
        marker = "inline-password-must-not-render"
        def bind(doc):
            for entry in doc.values():
                if isinstance(entry, dict) and "db" in entry:
                    entry["db"].update(existingSecret="plane-db", secretKey="connection")
                    if "dsn" in entry["db"]:
                        entry["db"]["dsn"] = f"postgres://user:{marker}@postgresql/plane"
        result = self.render_all(bind)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn(marker, result.stdout)
        checked = 0
        for doc in yaml.safe_load_all(result.stdout):
            if not doc or doc["kind"] not in ("Deployment", "StatefulSet"):
                continue
            for container in doc["spec"]["template"]["spec"]["containers"]:
                for env in container.get("env", []):
                    if env["name"] == "DB_DSN":
                        checked += 1
                        self.assertNotIn("value", env)
                        self.assertEqual(env["valueFrom"]["secretKeyRef"],
                                         {"name": "plane-db", "key": "connection"})
        self.assertGreaterEqual(checked, 19)

    def test_legacy_inline_database_dsn_remains_supported(self):
        def legacy(doc):
            for name in ("apiserver", "app-market", "platform-api", "operation-worker"):
                doc[name]["db"].update(existingSecret="", dsn="postgres://legacy/plane")
        result = self.render_all(legacy)
        self.assertEqual(result.returncode, 0, result.stderr)
        for doc in yaml.safe_load_all(result.stdout):
            if doc and doc["kind"] == "Deployment" and doc["metadata"]["name"] in (
                "apiserver", "app-market", "platform-api", "operation-worker"
            ):
                env = next(e for e in doc["spec"]["template"]["spec"]["containers"][0]["env"]
                           if e["name"] == "DB_DSN")
                self.assertEqual(env["value"], "postgres://legacy/plane")
                self.assertNotIn("valueFrom", env)

    def test_every_component_declares_non_root_pod_security_context(self):
        """Scratch images run as a non-root UID, so a Secret volume mounted 0400
        is root-owned and unreadable. Each component must set the pod security
        context that makes its mounted Secrets readable without loosening modes."""
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        checked = 0
        for doc in docs:
            if doc["kind"] not in ("Deployment", "StatefulSet"):
                continue
            pod = doc["spec"]["template"]["spec"]
            context = pod.get("securityContext")
            # The bundled PostgreSQL keeps its own image-defined UID.
            if doc["metadata"]["name"] == "postgresql":
                self.assertTrue(context, "postgresql must keep its own securityContext")
                continue
            self.assertIsNotNone(context, doc["metadata"]["name"])
            self.assertEqual(context["runAsNonRoot"], True, doc["metadata"]["name"])
            # Each component declares the UID its own image runs as; forcing the
            # scratch default onto a bundled image would break it.
            self.assertIsInstance(context["runAsUser"], int, doc["metadata"]["name"])
            self.assertGreater(context["runAsUser"], 0, doc["metadata"]["name"])
            self.assertEqual(context["fsGroup"], context["runAsGroup"],
                             doc["metadata"]["name"])
            self.assertEqual(context["fsGroupChangePolicy"], "OnRootMismatch",
                             doc["metadata"]["name"])
            # The scratch Go images all run as the non-root default; the bundled
            # nginx image keeps its own UID.
            expected_uid = {"portal": 101, "nats": 1000}.get(doc["metadata"]["name"], 65532)
            self.assertEqual(context["runAsUser"], expected_uid,
                             doc["metadata"]["name"])
            # Mounted Secrets stay group-only readable; the fsGroup, not the mode,
            # is what makes them readable by the non-root owner.
            for volume in pod.get("volumes", []):
                if "secret" in volume:
                    self.assertEqual(volume["secret"]["defaultMode"], 0o440,
                                     doc["metadata"]["name"])
            checked += 1
        self.assertGreaterEqual(checked, 15)

    def test_core_probes_separate_liveness_from_dependency_readiness(self):
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        core = {"apiserver", "app-market", "platform-api", "operation-worker", "extension-controller"}
        for doc in yaml.safe_load_all(result.stdout):
            if not doc or doc["kind"] != "Deployment" or doc["metadata"]["name"] not in core:
                continue
            container = doc["spec"]["template"]["spec"]["containers"][0]
            port_names = {p["name"] for p in container["ports"]}
            for probe in ("startupProbe", "readinessProbe", "livenessProbe"):
                self.assertIn(container[probe]["httpGet"]["port"], port_names)
            self.assertEqual(container["livenessProbe"]["httpGet"]["path"], "/livez")
            self.assertEqual(container["readinessProbe"]["httpGet"]["path"], "/readyz")
            startup = container["startupProbe"]
            self.assertGreaterEqual(startup["periodSeconds"] * startup["failureThreshold"], 240)

    def test_core_probes_can_be_disabled_for_older_images(self):
        def disable(doc):
            for name in ("apiserver", "app-market", "platform-api", "operation-worker", "extension-controller"):
                doc[name]["probes"] = {"enabled": False}
        result = self.render_all(disable)
        self.assertEqual(result.returncode, 0, result.stderr)
        for doc in yaml.safe_load_all(result.stdout):
            if doc and doc["kind"] == "Deployment":
                container = doc["spec"]["template"]["spec"]["containers"][0]
                self.assertNotIn("startupProbe", container)

    def test_portal_non_root_nginx_uses_writable_temp_paths(self):
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        deployment = next(d for d in docs if d["kind"] == "Deployment"
                          and d["metadata"]["name"] == "portal")
        config = next(d["data"] for d in docs if d["kind"] == "ConfigMap"
                      and d["metadata"]["name"] == "portal-nginx")
        pod = deployment["spec"]["template"]["spec"]
        container = pod["containers"][0]
        mounts = {m["mountPath"]: m for m in container["volumeMounts"]}
        self.assertEqual(mounts["/etc/nginx/nginx.conf"]["subPath"], "nginx.conf")
        self.assertIn("/etc/nginx/nginx.conf", container["command"])
        volume = next(v for v in pod["volumes"] if v["name"] == mounts["/tmp"]["name"])
        self.assertEqual(volume["emptyDir"]["sizeLimit"], "64Mi")
        self.assertIn("pid /tmp/nginx.pid;", config["nginx.conf"])
        for kind in ("client_body", "proxy", "fastcgi", "uwsgi", "scgi"):
            self.assertIn(f"{kind}_temp_path /tmp/{kind};", config["nginx.conf"])
        self.assertIn("listen 8080;", config["default.conf"])
        self.assertEqual(container["ports"][0]["containerPort"], 8080)

    def test_apiserver_stages_private_key_for_the_non_root_user(self):
        """pkg/iam requires an owner-only signing key, but the Secret is mounted
        root-owned while the scratch image runs as a non-root UID. An init
        container must hand the key over at 0400 without weakening the check."""
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        deployment = next(d for d in docs if d and d["kind"] == "Deployment"
                          and d["metadata"]["name"] == "apiserver")
        pod = deployment["spec"]["template"]["spec"]
        self.assertEqual(pod["securityContext"]["runAsNonRoot"], True)
        self.assertEqual(pod["securityContext"]["runAsUser"], 65532)
        self.assertEqual(pod["securityContext"]["fsGroup"], 65532)
        init = pod["initContainers"]
        self.assertEqual(len(init), 1)
        self.assertEqual(init[0]["name"], "stage-signing-key")
        self.assertIn("@sha256:", init[0]["image"])
        # The init container must run as root: only root can chown the staged file.
        self.assertEqual(init[0]["securityContext"]["runAsNonRoot"], False)
        self.assertIn("chown 65532:65532", " ".join(init[0]["command"]))
        self.assertIn("chmod 0400", " ".join(init[0]["command"]))
        volumes = {v["name"]: v for v in pod["volumes"]}
        self.assertIn("staged-signing-key", volumes)
        self.assertEqual(volumes["staged-signing-key"]["emptyDir"]["medium"], "Memory")
        # The application must read the staged copy, not the root-owned Secret.
        mounts = {m["name"]: m for m in pod["containers"][0]["volumeMounts"]}
        self.assertEqual(mounts["staged-signing-key"]["mountPath"], "/var/run/hnb-signing")
        self.assertNotIn("active-signing-key", mounts)
        self.assertEqual(volumes["active-signing-key"]["secret"]["defaultMode"], 0o440)

    def test_app_market_gets_a_writable_bounded_transfer_staging_volume(self):
        """A scratch image ships no writable /tmp, so artifact transfer staging
        must be an explicit bounded volume rather than a host path."""
        result = self.render_all()
        self.assertEqual(result.returncode, 0, result.stderr)
        docs = [d for d in yaml.safe_load_all(result.stdout) if d]
        deployment = next(d for d in docs if d and d["kind"] == "Deployment"
                          and d["metadata"]["name"] == "app-market")
        pod = deployment["spec"]["template"]["spec"]
        container = pod["containers"][0]
        env = {e["name"]: e for e in container["env"]}
        staging = env["ARTIFACT_TRANSFER_STAGING_DIR"]["value"]
        self.assertEqual(staging, "/var/lib/hnb/transfer")
        self.assertNotIn("/tmp", staging)
        mounts = {m["name"]: m for m in container["volumeMounts"]}
        self.assertEqual(mounts["transfer-staging"]["mountPath"], staging)
        volumes = {v["name"]: v for v in pod["volumes"]}
        self.assertEqual(volumes["transfer-staging"]["emptyDir"]["sizeLimit"], "1Gi")

    def test_missing_placeholder_and_bad_digest_are_rejected(self):
        def drop(doc):
            for name in doc:
                if name != "global":
                    doc[name]["image"].pop("digest", None)

        def placeholder(doc):
            for name in doc:
                if name != "global":
                    doc[name]["image"]["digest"] = "sha256:" + "0" * 64

        def latest(doc):
            for name in doc:
                if name != "global":
                    doc[name]["image"]["digest"] = "latest"

        for mutate, needle in ((drop, "sha256 digest"), (placeholder, "placeholder"),
                               (latest, "does not match pattern")):
            result = self.render_all(mutate)
            self.assertNotEqual(result.returncode, 0, f"accepted: {needle}")
            self.assertIn(needle, result.stdout + result.stderr)

    def test_global_registry_retargets_all_images(self):
        def retarget(doc):
            doc["global"]["imageRegistry"] = "harbor.hnb.local/hnb"
        result = self.render_all(retarget)
        self.assertEqual(result.returncode, 0, result.stderr)
        images = [line.split("image:", 1)[1].strip().strip('"')
                  for line in result.stdout.splitlines() if line.strip().startswith("image:")]
        self.assertTrue(images)
        for image in images:
            self.assertTrue(image.startswith("harbor.hnb.local/hnb/"), image)
            self.assertIn("@sha256:", image)

    def test_portal_delivery_resources(self):
        docs = list(yaml.safe_load_all(self.render_all(lambda d: d["portal"].update(
            {"ingress": {"enabled": True, "host": "portal.hnb.test",
                         "tlsSecret": "fixture-tls", "className": "nginx"}})).stdout))
        kinds = {d["kind"] for d in docs if d and d["metadata"]["name"] == "portal"}
        self.assertEqual(kinds, {"Deployment", "Service", "Ingress"})
        ingress = next(d for d in docs if d and d["kind"] == "Ingress")
        self.assertEqual(ingress["spec"]["rules"][0]["host"], "portal.hnb.test")
        self.assertEqual(ingress["spec"]["tls"][0]["secretName"], "fixture-tls")


class BundledInfrastructureTests(unittest.TestCase):
    """Bundled PG/NATS persist to disk and keep one identity per owned plane."""

    PLANES = {
        "platform": {"database": "hnb_platform", "role": "hnb_platform"},
        "market": {"database": "hnb_market", "role": "hnb_market"},
        "artifact": {"database": "hnb_artifact", "role": "hnb_artifact"},
    }

    def render_infra(self, component, extra=None):
        """Render the parent chart with every component digest-pinned."""
        doc = WholeChartDigestTests().render_values()
        # Isolate the bundled component under test.
        for name in ("postgresql", "nats"):
            doc[name]["enabled"] = False
        doc[component] = {"enabled": True,
                          "image": {"repository": f"hnb/{component}", "digest": DIGEST},
                          "storage": {"size": "20Gi", "className": "fixture-sc"},
                          "planes": {p: dict(spec) for p, spec in self.PLANES.items()}}
        if component == "postgresql":
            doc[component]["adminSecret"] = "fixture-admin"
            for p in self.PLANES: doc[component]["planes"][p]["existingSecret"]="fixture-"+p+"-pg"
        else:
            doc[component]["tls"]={"existingSecret":"fixture-nats-tls"}
            doc[component]["planes"]={p:{"existingSecret":"fixture-"+p+"-nats"} for p in self.PLANES}

        for key, value in (extra or {}).items():
            doc[component][key] = value
        path = CHART / "test-infra.yaml"
        path.write_text(yaml.safe_dump(doc, sort_keys=True))
        try:
            result = subprocess.run(["helm", "template", "fixture", str(CHART), "-f", str(path)],
                                    capture_output=True, text=True, timeout=120)
        finally:
            path.unlink(missing_ok=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.last_stdout = result.stdout
        return list(yaml.safe_load_all(result.stdout))

    def test_postgresql_persists_and_isolates_planes(self):
        docs = self.render_infra("postgresql")
        sts = next(d for d in docs if d and d["kind"] == "StatefulSet")
        pvc = next(d for d in docs if d and d["kind"] == "PersistentVolumeClaim")
        self.assertEqual(pvc["spec"]["storageClassName"], "fixture-sc")
        volumes = {v["name"]: v for v in sts["spec"]["template"]["spec"]["volumes"]}
        self.assertIn("persistentVolumeClaim", volumes["data"])
        script = self.last_stdout
        # PG runs its script after starting a temporary server, not in a
        # Kubernetes initContainer waiting for its own main container.
        self.assertNotIn("initContainers", sts["spec"]["template"]["spec"])
        self.assertIn("CREATE ROLE", script)
        self.assertIn("CREATE DATABASE", script)
        self.assertIn("REVOKE ALL ON DATABASE %I FROM PUBLIC", script)
        self.assertNotIn("/dev/urandom", script)
        self.assertEqual(volumes["admin-credential"]["secret"]["secretName"],"fixture-admin")
        for plane in self.PLANES:
            self.assertEqual(volumes[plane+"-credential"]["secret"]["secretName"],"fixture-"+plane+"-pg")
        pg_env = {e["name"]: e for e in sts["spec"]["template"]["spec"]["containers"][0]["env"]}
        self.assertEqual(pg_env["POSTGRES_PASSWORD_FILE"]["value"],"/var/run/hnb-pg-admin/password")

    def test_nats_jetstream_persists_to_disk(self):
        docs = self.render_infra("nats")
        sts = next(d for d in docs if d and d["kind"] == "StatefulSet" and d["metadata"]["name"] == "nats")
        config = next(d for d in docs if d and d["kind"] == "ConfigMap" and d["metadata"]["name"] == "nats-config")
        pvc = next(d for d in docs if d and d["kind"] == "PersistentVolumeClaim")
        self.assertEqual(pvc["spec"]["storageClassName"], "fixture-sc")
        self.assertIn("jetstream", config["data"]["nats.conf"])
        self.assertIn("store_dir", config["data"]["nats.conf"])
        self.assertNotIn("initContainers",sts["spec"]["template"]["spec"])
        for plane in self.PLANES:
            self.assertIn("$HNB_"+plane.upper()+"_PASSWORD",config["data"]["nats.conf"])
        env={e["name"]:e for e in sts["spec"]["template"]["spec"]["containers"][0]["env"]}
        for plane in self.PLANES:
            self.assertEqual(env["HNB_"+plane.upper()+"_PASSWORD"]["valueFrom"]["secretKeyRef"]["name"],"fixture-"+plane+"-nats")

    def test_no_credential_material_in_rendered_output(self):
        for component in ("postgresql", "nats"):
            with self.subTest(component=component):
                rendered = yaml.safe_dump(self.render_infra(component))
                for pattern in (r"change-me", r"hunter2", r"BEGIN [A-Z ]*PRIVATE KEY",
                                r"postgres(?:ql)?://[^\s]*:[^\s@]*@"):
                    self.assertNotRegex(rendered, pattern)

    def test_bundled_infrastructure_requires_its_secret_refs(self):
        for component, missing in (("postgresql", "postgresql.adminSecret"),):
            result = subprocess.run(
                ["helm", "template", "fixture", str(CHART),
                 "--set", f"{component}.enabled=true",
                 "--set", f"{component}.image.digest={DIGEST}",
                 "--set", f"{missing}="],
                capture_output=True, text=True, timeout=60)
            self.assertNotEqual(result.returncode, 0, f"rendered without {missing}")


class CandidateChartTests(unittest.TestCase):
    def render(self, component, overrides=None):
        result = command(component, overrides)
        self.assertEqual(result.returncode, 0, result.stderr)
        return list(yaml.safe_load_all(result.stdout))

    def test_lint_and_digest_pinning(self):
        for component in ("portal", "plugin-worker", "runtime-target-lifecycle-provider"):
            with self.subTest(component=component):
                lint = command(component, action="lint")
                self.assertEqual(lint.returncode, 0, lint.stdout + lint.stderr)
                docs = self.render(component)
                deployment = next(d for d in docs if d["kind"] == "Deployment")
                container = deployment["spec"]["template"]["spec"]["containers"][0]
                self.assertEqual(container["image"], f"hnb/{component}@{DIGEST}")
                for invalid in ("", "latest", "sha256:bad", "sha256:" + "0" * 64):
                    self.assertNotEqual(command(component, {"image.digest": invalid}).returncode, 0)

    def test_plugin_worker_secret_and_tls_wiring(self):
        docs = self.render("plugin-worker")
        container = docs[0]["spec"]["template"]["spec"]["containers"][0]
        env = {v["name"]: v for v in container["env"]}
        self.assertEqual(env["DB_DSN"]["valueFrom"]["secretKeyRef"]["name"], "fixture-db")
        for key in ("NATS_TLS_CERT_FILE", "NATS_TLS_KEY_FILE", "NATS_TLS_CA_FILE"):
            self.assertTrue(env[key]["value"].startswith("/var/run/hnb-nats/"))
        self.assertNotEqual(command("plugin-worker", {"db.existingSecret": ""}).returncode, 0)

    def test_lifecycle_provider_requires_service_identity(self):
        """Step execution must never be reachable without a service identity."""
        docs = self.render("runtime-target-lifecycle-provider",
                           {"identity.existingManifestSecret": "fixture-manifest"})
        deployment = next(d for d in docs if d["kind"] == "Deployment")
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        env = {v["name"]: v.get("value") for v in container["env"]}
        self.assertEqual(env["API_TOKEN_AUDIENCE"], "hnb-runtime-target-lifecycle-provider")
        self.assertTrue(env["API_TOKEN_ISSUER"].startswith("https://"))
        self.assertEqual(env["API_TOKEN_KEY_MANIFEST_FILE"], "/var/run/hnb-identity/manifest.json")
        spec = deployment["spec"]["template"]["spec"]
        volumes = {v["name"]: v for v in spec["volumes"]}
        self.assertIn("identity-keys", volumes)
        self.assertEqual(volumes["identity-keys"]["secret"]["secretName"], "fixture-manifest")
        self.assertEqual(volumes["identity-keys"]["secret"]["defaultMode"], 0o440)
        mounts = {m["name"]: m["mountPath"] for m in container["volumeMounts"]}
        self.assertEqual(mounts["identity-keys"], "/var/run/hnb-identity")
        for missing in ("identity.existingManifestSecret", "identity.issuer", "identity.audience"):
            self.assertNotEqual(
                command("runtime-target-lifecycle-provider",
                        {"identity.existingManifestSecret": "fixture-manifest", missing: ""}).returncode, 0,
                f"provider rendered without {missing}")

    def test_lifecycle_service_and_profile(self):
        docs = self.render("runtime-target-lifecycle-provider")
        service = next(d for d in docs if d["kind"] == "Service")
        self.assertEqual(service["spec"]["ports"][0]["port"], 18082)
        deployment = next(d for d in docs if d["kind"] == "Deployment")
        container = deployment["spec"]["template"]["spec"]["containers"][0]
        env = {v["name"]: v for v in container["env"]}
        self.assertEqual(env["PROVIDER_ID"]["value"], "runtime-target.lifecycle.kubernetes")
        self.assertEqual(env["LISTEN_ADDRESS"]["value"], ":18082")
        self.assertNotIn("NATS_URL", env)  # The provider does not use NATS.
        self.assertNotEqual(command("runtime-target-lifecycle-provider", {"providerId": "fake"}).returncode, 0)

    def test_portal_proxy_and_tls_entry(self):
        docs = self.render("portal", {"ingress.enabled": "true", "ingress.host": "fixture.invalid", "ingress.tlsSecret": "fixture-tls"})
        config = next(d for d in docs if d["kind"] == "ConfigMap")
        self.assertIn("root /usr/share/nginx/html/shell;", config["data"]["default.conf"])
        self.assertIn("proxy_pass http://apiserver:8080;", config["data"]["default.conf"])
        ingress = next(d for d in docs if d["kind"] == "Ingress")
        self.assertEqual(ingress["spec"]["tls"][0]["secretName"], "fixture-tls")
        self.assertNotEqual(command("portal", {"apiBaseURL": "http://api; injected"}).returncode, 0)


if __name__ == "__main__":
    unittest.main(verbosity=2)
