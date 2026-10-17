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
	address := cp.Host
	if spec.AdvertiseAddress != "" {
		address = spec.AdvertiseAddress
	}
	extra := ""
	if spec.ImageRepository != "" {
		extra = "imageRepository: " + spec.ImageRepository + "\n"
	}
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
%snetworking:
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
		kubeadmAPIVersion, spec.DesiredVersion, spec.Endpoint(), extra,
		podCIDR, svcCIDR, dns,
		sans.String(),
		address, bindPort, sanitizeNodeName(cp), address,
	)
}

// PrereqScript installs kubeadm/kubelet/kubectl + containerd on a node from
// the pkgs.k8s.io minor channel. Existing proxy/runtime policy is preserved.
func PrereqScript(spec ProvisionSpec) string {
	channel := "v" + minorVersion(spec.DesiredVersion)
	return fmt.Sprintf(`set -eu
expected_version='%s'
sysctl -w net.ipv4.ip_forward=1 >/dev/null
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
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -yq >/dev/null 2>&1
%s  apt-get install -yq $apt_bootstrap_packages >/dev/null 2>&1
  mkdir -p /etc/apt/keyrings
  curl -fsSL https://pkgs.k8s.io/core:/stable:/%s/deb/Release.key | gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg >/dev/null 2>&1
  echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/%s/deb/ /" > /etc/apt/sources.list.d/kubernetes.list
  apt-get update -yq >/dev/null 2>&1
  package_version=$(apt-cache madison kubeadm | awk -v expected="${expected_version#v}-" 'index($3, expected) == 1 { print $3; exit }')
  if [ -z "$package_version" ]; then
    echo "requested kubeadm patch version is unavailable in the APT channel" >&2
    exit 1
  fi
  apt-get install -yq "kubelet=$package_version" "kubeadm=$package_version" "kubectl=$package_version" containerd >/dev/null 2>&1
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
`, "v"+strings.TrimPrefix(spec.DesiredVersion, "v"), aptBootstrapPackagesScript(), channel, channel, channel, channel)
}

// containerdImportMerge is the POSIX awk program that registers a configuration
// import in an existing containerd config.toml. It deliberately uses only awk and
// core shell utilities so bootstrap does not depend on an interpreter that the
// distribution image may not ship. An unrecognised structure fails closed
// instead of rewriting administrator configuration.
const containerdImportMerge = `
function append_entry(line) {
  sub(/[ \t]+$/, "", line)
  if (line ~ /,[ \t]*\]$/) sub(/,[ \t]*\]$/, "]", line)
  if (line ~ /\[[ \t]*\]$/) { sub(/\[[ \t]*\]$/, "[\"" entry "\"]", line); return line }
  sub(/\]$/, ", \"" entry "\"]", line)
  return line
}
{
  if (state == 2) { print; next }
  if (state == 1) {
    if ($0 ~ /\]/) { print append_entry($0); state = 2 } else { print }
    next
  }
  if ($0 ~ /^[ \t]*imports[ \t]*=/) {
    if ($0 ~ /\]/) { print append_entry($0); state = 2 } else { print; state = 1 }
    next
  }
  print
}
END { if (state != 2) exit 3 }
`

// RuntimeConfigScript adds a focused import without replacing administrator
// runtime settings. An explicit sandbox image avoids registry.k8s.io fallback.
func RuntimeConfigScript(spec ProvisionSpec) string {
	if spec.NetworkPlugin != "cilium" && spec.SandboxImage == "" {
		return "true"
	}
	sandbox := ""
	if spec.SandboxImage != "" {
		sandbox = "  sandbox_image = '" + spec.SandboxImage + "'\n"
	}
	return fmt.Sprintf(`set -eu
command -v awk >/dev/null
# HNB_CONTAINERD_DIR only exists so the merge can be exercised against a fixture
# directory; it is never set during a real bootstrap.
containerd_dir="${HNB_CONTAINERD_DIR:-/etc/containerd}"
mkdir -p "$containerd_dir"
cat >"$containerd_dir/hnb-bootstrap.toml" <<'CFG'
version = 2
[plugins."io.containerd.grpc.v1.cri"]
%s[plugins."io.containerd.grpc.v1.cri".cni]
  bin_dir = "/opt/cni/bin"
  conf_dir = "/etc/cni/net.d"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
CFG
import_path="$containerd_dir/hnb-bootstrap.toml"
config="$containerd_dir/config.toml"
touch "$config"
if grep -qF "\"$import_path\"" "$config"; then
  :
elif grep -qE '^[ \t]*imports[ \t]*=' "$config"; then
  tmp=$(mktemp "$containerd_dir/config.toml.hnb.XXXXXX")
  awk -v entry="$import_path" '%s' "$config" >"$tmp" || { rm -f "$tmp"; echo 'cannot merge containerd imports' >&2; exit 1; }
  cat "$tmp" >"$config" && rm -f "$tmp"
else
  tmp=$(mktemp "$containerd_dir/config.toml.hnb.XXXXXX")
  { printf 'imports = ["%%s"]\n' "$import_path"; cat "$config"; } >"$tmp"
  cat "$tmp" >"$config" && rm -f "$tmp"
fi
containerd config dump >/dev/null
systemctl restart containerd
systemctl is-active --quiet containerd
if [ -f /var/lib/kubelet/config.yaml ]; then
  systemctl restart kubelet
fi
`, sandbox, containerdImportMerge)
}

// InitScript drives kubeadm init and prints a machine-parseable join token/hash.
func InitScript(spec ProvisionSpec, cp NodeSpec) string {
	skip := ""
	if spec.NetworkPlugin == "cilium" {
		skip = " --skip-phases=addon/kube-proxy"
	}
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
if ! kubeadm init --config=/etc/kubernetes/kubeadm-config.yaml%s > /var/log/kubeadm-init.log 2>&1; then
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
`, ControlPlaneConfig(spec, cp), skip, spec.Endpoint())
}

// NetworkScript installs the explicitly selected datapath. Cilium replaces
// kube-proxy and uses Kubernetes PodCIDRs instead of a separate IPAM pool.
// ciliumChartEnvoyTag is the Envoy image tag pinned by the Cilium 1.20.1 chart
// (quay.io/cilium/cilium-envoy). It is not derived from the Cilium version, so
// upgrading the chart can change it; verify it whenever the chart version moves.
const ciliumChartEnvoyTag = "v1.37.5-1786810558-766ccfb37260a43e9d228837aa84ce3faf9f64e7"

func NetworkScript(spec ProvisionSpec) string {
	if spec.NetworkPlugin != "cilium" {
		return CNIScript("")
	}
	address := spec.ControlPlaneNodes()[0].Host
	if spec.AdvertiseAddress != "" {
		address = spec.AdvertiseAddress
	}
	podCIDR := spec.PodCIDR
	if podCIDR == "" {
		podCIDR = "10.244.0.0/16"
	}
	version := strings.TrimPrefix(spec.NetworkVersion, "v")
	// Every node image, including the CNI data path, is resolved through the
	// configured platform repository. Reaching an upstream public registry
	// implicitly would break air-gapped installs and split the trust boundary.
	cniRepository := spec.CNIImageRepository
	if cniRepository == "" {
		cniRepository = spec.ImageRepository
	}
	// Harbor serves charts as OCI artifacts, not a classic index.yaml repository,
	// so the chart reference is an explicit oci:// project with a pinned version.
	chartOrigin := spec.HelmChartRepository
	chartFlags := ""
	for _, prefix := range []string{"http://", "https://"} {
		if strings.HasPrefix(chartOrigin, prefix) {
			if prefix == "http://" {
				chartFlags = " --plain-http"
			}
			chartOrigin = strings.TrimPrefix(chartOrigin, prefix)
			break
		}
	}
	chartRef := "oci://" + strings.TrimSuffix(chartOrigin, "/") + "/cilium"
	// Cilium ships an Envoy sidecar whose image tag is fixed by the chart, not by
	// the Cilium version. It must be redirected to the platform repository too,
	// otherwise the datapath still reaches a public upstream.
	envoyTag := ciliumChartEnvoyTag
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
if kubectl -n kube-flannel get daemonset kube-flannel-ds >/dev/null 2>&1; then
  echo 'cannot install Cilium over an existing Flannel datapath' >&2
  exit 1
fi
helm upgrade --install cilium '%s' --version '%s'%s --namespace kube-system \
  --set kubeProxyReplacement=true --set routingMode=native --set autoDirectNodeRoutes=true --set bpf.masquerade=true \
  --set ipam.mode=kubernetes --set ipv4NativeRoutingCIDR='%s' \
  --set k8sServiceHost='%s' --set k8sServicePort=6443 --set operator.replicas=1 \
  --set image.override='%s/cilium:v%s' \
  --set operator.image.override='%s/cilium-operator:v%s' \
  --set image.useDigest=false \
  --set operator.image.useDigest=false \
  --set envoy.image.override='%s/cilium-envoy:%s' \
  --set envoy.image.useDigest=false
`, chartRef, version, chartFlags, podCIDR, address, cniRepository, version, cniRepository, version, cniRepository, envoyTag)
}

// JoinScript idempotently joins a worker (or additional control-plane) node.
func JoinScript(spec ProvisionSpec, node NodeSpec, token, caHash string) string {
	cmd := fmt.Sprintf(
		"kubeadm join %s --token %s --discovery-token-ca-cert-hash %s --node-name %s",
		spec.Endpoint(), token, caHash, sanitizeNodeName(node),
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

// CNIScript installs the Flannel pod network after kubeadm init.
const defaultFlannelManifest = "https://raw.githubusercontent.com/flannel-io/flannel/v0.25.2/Documentation/kube-flannel.yml"

func CNIScript(manifest string) string {
	if strings.TrimSpace(manifest) == "" {
		manifest = defaultFlannelManifest
	}
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
if kubectl -n kube-system get daemonset cilium >/dev/null 2>&1; then
  echo 'cannot install Flannel over an existing Cilium datapath' >&2
  exit 1
fi
kubectl get ds -n kube-flannel kube-flannel-ds >/dev/null 2>&1 && { echo "__CNI_PRESENT__"; exit 0; }
kubectl apply -f '%s'
sleep 5
exit 0
`, manifest)
}

// AcceptanceScript gates success on observed API, node and CNI readiness.
// Cold CNI image pulls may need a retry; allow ten minutes per readiness gate.
func AcceptanceScript(spec ProvisionSpec) string {
	networkCheck := "kubectl -n kube-flannel rollout status daemonset/kube-flannel-ds --timeout=600s"
	if spec.NetworkPlugin == "cilium" {
		networkCheck = "kubectl -n kube-system rollout status daemonset/cilium --timeout=600s\nkubectl -n kube-system rollout status deployment/cilium-operator --timeout=600s"
	}
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl get --raw=/readyz >/dev/null
test "$(kubectl get nodes -o name | wc -l)" -eq %d
kubectl wait --for=condition=Ready nodes --all --timeout=600s
%s
kubectl -n kube-system rollout status deployment/coredns --timeout=600s
echo '__HNB_CLUSTER_ACCEPTED__'
`, len(spec.Nodes), networkCheck)
}

// NodesReadyScript accepts a node add by observing the named nodes turn Ready
// on the control plane; the join exit status alone is not acceptance evidence.
func NodesReadyScript(nodeNames []string) string {
	var waits strings.Builder
	for _, name := range nodeNames {
		fmt.Fprintf(&waits, "kubectl wait --for=condition=Ready node/%s --timeout=600s\n", ShQuote(name))
	}
	return fmt.Sprintf(`set -eu
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl get --raw=/readyz >/dev/null
%secho '__HNB_NODES_READY__'
`, waits.String())
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
