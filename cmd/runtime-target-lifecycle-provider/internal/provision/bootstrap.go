package provision

import (
	"fmt"
	"regexp"
	"strings"
)

// ControlPlaneConfig renders the kubeadm ClusterConfiguration for a
// control-plane node. It is a pure function so the YAML shape is unit-testable.
func ControlPlaneConfig(spec ProvisionSpec, cp NodeSpec) string {
	podCIDR := spec.PodCIDR
	if podCIDR == "" {
		podCIDR = "10.244.0.0/16"
	}
	svcCIDR := spec.ServiceCIDR
	if svcCIDR == "" {
		svcCIDR = "10.96.0.0/12"
	}
	dns := spec.ClusterDomain
	if dns == "" {
		dns = "cluster.local"
	}
	seen := map[string]bool{"127.0.0.1": true, "10.96.0.1": true}
	var sans strings.Builder
	// include every node IP plus the endpoint host so kubeconfig download and
	// joins from any node resolve the apiserver cert.
	for _, n := range spec.Nodes {
		if !seen[n.Host] {
			seen[n.Host] = true
			sans.WriteString("    - " + n.Host + "\n")
		}
	}
	if epHost, _, _ := strings.Cut(spec.Endpoint(), ":"); epHost != "" && !seen[epHost] {
		seen[epHost] = true
		sans.WriteString("    - " + epHost + "\n")
	}

	return fmt.Sprintf(`apiVersion: %s
kind: ClusterConfiguration
kubernetesVersion: %s
controlPlaneEndpoint: %s
networking:
  podSubnet: %s
  serviceSubnet: %s
  dnsDomain: %s
apiServer:
  certSANs:
    - 127.0.0.1
    - 10.96.0.1
%s---
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: %s
  bindPort: %d
nodeRegistration:
  name: %s
  kubeletExtraArgs:
    - name: node-ip
      value: %s
  taints:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
`,
		kubeadmAPIVersion, spec.DesiredVersion, spec.Endpoint(),
		podCIDR, svcCIDR, dns,
		sans.String(),
		cp.Host, bindPort, sanitizeNodeName(cp), cp.Host,
	)
}

// PrereqScript installs kubeadm/kubelet/kubectl + containerd on a node from
// the pkgs.k8s.io minor channel. Existing proxy/runtime policy is preserved.
func PrereqScript(spec ProvisionSpec) string {
	channel := "v" + minorVersion(spec.DesiredVersion)
	return fmt.Sprintf(`set -eu
expected_version='%s'
if [ "$(wc -l < /proc/swaps)" -gt 1 ]; then
  echo 'swap must be disabled by the node administrator before bootstrap' >&2
  exit 1
fi
if command -v kubeadm >/dev/null 2>&1; then
  test "$(kubeadm version -o short)" = "$expected_version"
  systemctl is-active --quiet containerd
  echo "__PREREQ_PRESENT__"
  exit 0
fi
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -yq >/dev/null 2>&1
  apt-get install -yq apt-transport-https ca-certificates curl gpg >/dev/null 2>&1
  mkdir -p /etc/apt/keyrings
  curl -fsSL https://pkgs.k8s.io/core:/stable:/%s/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg >/dev/null 2>&1
  echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/%s/deb/ /" > /etc/apt/sources.list.d/kubernetes.list
  apt-get update -yq >/dev/null 2>&1
  apt-get install -yq kubelet kubeadm kubectl containerd >/dev/null 2>&1
  echo "__PREREQ_APT__"
elif command -v yum >/dev/null 2>&1; then
  cat >/etc/yum.repos.d/kubernetes.repo <<'REPO'
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/%s/rpm/
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/%s/rpm/repodata/repomd.xml.key
REPO
  yum install -y kubelet kubeadm kubectl containerd --disableexcludes=kubernetes >/dev/null 2>&1
  echo "__PREREQ_YUM__"
else
  echo "unsupported package manager" >&2
  exit 1
fi
systemctl enable --now kubelet containerd >/dev/null 2>&1
# 不覆盖已有 containerd 配置：发行版/管理员可能已写好 sandbox_image、
# 快照器、base_runtime_spec 等关键项，覆盖会丢弃它们导致 CRI 无法启动
# 静态 Pod（例如 kubelet 需要用 sandbox/pause 镜像启动 infra 容器）。
if [ ! -f /etc/containerd/config.toml ]; then
  mkdir -p /etc/containerd
  cat > /etc/containerd/config.toml <<'CTR2'
version = 2
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
[plugins."io.containerd.grpc.v1.cri"]
  sandbox_image = "registry.k8s.io/pause:3.10"
CTR2
fi
systemctl daemon-reload >/dev/null 2>&1
systemctl restart containerd >/dev/null 2>&1
test "$(kubeadm version -o short)" = "$expected_version"
exit 0
`, "v"+strings.TrimPrefix(spec.DesiredVersion, "v"), channel, channel, channel, channel)
}

// InitScript drives kubeadm init on the first control-plane node and prints a
// machine-parseable join bootstrap token + CA hash.
func InitScript(spec ProvisionSpec, cp NodeSpec) string {
	return fmt.Sprintf(`set -eu
if [ -f /etc/kubernetes/admin.conf ]; then
  export KUBECONFIG=/etc/kubernetes/admin.conf
  kubectl get --raw=/readyz >/dev/null
  kubeadm token create --print-join-command
  exit 0
fi
mkdir -p /etc/kubernetes
cat > /etc/kubernetes/kubeadm-config.yaml <<'CFG'
%s
CFG
if ! command -v kubeadm >/dev/null 2>&1; then
  echo "kubeadm not installed; run prereq first" >&2
  exit 1
fi
if ! kubeadm init --config=/etc/kubernetes/kubeadm-config.yaml > /var/log/kubeadm-init.log 2>&1; then
  tail -n 40 /var/log/kubeadm-init.log >&2
  exit 1
fi
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl get nodes -o wide
# emit an easily-parsed join bootstrap
openssl x509 -pubkey -in /etc/kubernetes/pki/ca.crt | openssl rsa -pubin -outform der 2>/dev/null | openssl dgst -sha256 -hex | sed 's/^.* //' > /var/log/ca-sha
echo "__HNB_JOIN_TOKEN__=$(kubeadm token create 2>/dev/null)"
echo "__HNB_CA_HASH__=sha256:$(cat /var/log/ca-sha)"
# bootstrap CLI kubeconfig so workers can reach the apiserver
sed -i "s#server: https://[0-9.:]*#server: https://%s#g" /etc/kubernetes/admin.conf
exit 0
`, ControlPlaneConfig(spec, cp), spec.Endpoint())
}

// JoinScript idempotently joins a worker (or additional control-plane) node.
func JoinScript(spec ProvisionSpec, node NodeSpec, token, caHash string) string {
	cmd := fmt.Sprintf(
		"kubeadm join %s --token %s --discovery-token-ca-cert-hash %s",
		spec.Endpoint(), token, caHash,
	)
	if node.Role == RoleControlPlane {
		cmd += " --control-plane"
	}
	return fmt.Sprintf(`set -eu
if [ -f /etc/kubernetes/kubelet.conf ] && [ -f /etc/kubernetes/pki/ca.crt ]; then
  echo "__KUBEADM_ALREADY_JOINED__"
  exit 0
fi
if ! command -v kubeadm >/dev/null 2>&1; then
  echo "kubeadm not installed; run prereq first" >&2
  exit 1
fi
%s
echo "__KUBEADM_JOIN_OK__"
exit 0
`, cmd)
}

// CNIScript installs a pod network after kubeadm init. Defaults to Flannel;
// a custom manifest URL can be passed via Parameters[cniManifest].
const cniManifestParam = "cniManifest"
const defaultFlannelManifest = "https://raw.githubusercontent.com/flannel-io/flannel/v0.25.2/Documentation/kube-flannel.yml"

func CNIScript(manifest string) string {
	if strings.TrimSpace(manifest) == "" {
		manifest = defaultFlannelManifest
	}
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl get ds -n kube-flannel kube-flannel-ds >/dev/null 2>&1 && { echo "__CNI_PRESENT__"; exit 0; }
kubectl apply -f '%s'
sleep 5
exit 0
`, manifest)
}

// AcceptanceScript gates success on observed API, node and CNI readiness.
func AcceptanceScript(spec ProvisionSpec) string {
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl get --raw=/readyz >/dev/null
test "$(kubectl get nodes -o name | wc -l)" -eq %d
kubectl wait --for=condition=Ready nodes --all --timeout=300s
kubectl -n kube-flannel rollout status daemonset/kube-flannel-ds --timeout=300s
kubectl -n kube-system rollout status deployment/coredns --timeout=300s
echo '__HNB_CLUSTER_ACCEPTED__'
`, len(spec.Nodes))
}

// AdminKubeconfigScript prints the admin kubeconfig path so the provider can
// read and store it.
const AdminKubeconfigScript = `cat /etc/kubernetes/admin.conf 2>/dev/null || echo "__NO_ADMIN_CONF__"`

var (
	joinTokenPattern = regexp.MustCompile(`^(?:[a-z0-9]{2,3}\.)?([a-z0-9]{6}\.[a-z0-9]{16})$`)
	caHashPattern    = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	// joinLinePattern matches `kubeadm join <server> --token <tok> ... --discovery-token-ca-cert-hash sha256:...`.
	joinLinePattern = regexp.MustCompile(`kubeadm join [^\n]*--token ([a-z0-9]{6}\.[a-z0-9]{16})(?:[^\n]*--discovery-token-ca-cert-hash[ =]?([^\s]+))?`)
)

// ParseJoinDetails extracts token + CA hash from `kubeadm init` output lines.
// Falls back to the explicit __HNB_JOIN_TOKEN__ / __HNB_CA_HASH__ markers the
// InitScript prints when the standard join banner is not present.
func ParseJoinDetails(output string) (token, caHash string, err error) {
	for _, marker := range []struct {
		prefix string
		dst    *string
	}{
		{"__HNB_JOIN_TOKEN__=", &token},
		{"__HNB_CA_HASH__=", &caHash},
	} {
		for _, line := range strings.Split(output, "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), marker.prefix); ok {
				*marker.dst = strings.TrimSpace(v)
			}
		}
	}
	if token == "" || caHash == "" {
		for _, line := range strings.Split(output, "\n") {
			if m := joinLinePattern.FindStringSubmatch(line); len(m) >= 1 {
				if token == "" {
					token = m[1]
				}
				if caHash == "" && len(m) >= 3 && m[2] != "" {
					caHash = m[2]
				}
			}
			if caHash == "" {
				if h := caHashPattern.FindString(line); h != "" {
					caHash = h
				}
			}
		}
	}
	if token == "" || caHash == "" {
		return "", "", fmt.Errorf("kubeadm join token/hash not found in init output: %s", tail(output))
	}
	if !joinTokenPattern.MatchString(token) {
		return "", "", fmt.Errorf("invalid kubeadm bootstrap token %q", token)
	}
	return token, caHash, nil
}

// KubernetesVersionPrefix extracts the pinned "v1.31" style channel from a
// version like "v1.31.4" or "1.31.4".
func minorVersion(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// sanitizeNodeName maps a node ID/host to a valid DNS-1123 node name.
func sanitizeNodeName(n NodeSpec) string {
	name := strings.ToLower(strings.TrimSpace(n.ID))
	if name == "" {
		name = strings.ToLower(n.Host)
	}
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "node"
	}
	return name
}

// shortID returns the first 12 chars of a UUID-ish id, lowercased.
func shortID(id string) string {
	id = strings.ReplaceAll(strings.ToLower(id), "-", "")
	if len(id) > 12 {
		return id[:12]
	}
	if id == "" {
		return "clstr"
	}
	return id
}
