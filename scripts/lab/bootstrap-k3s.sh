#!/usr/bin/env bash
# Dedicated S2/S3 lab only; run as root on one of the declared clean hosts.
# For an agent, feed the server's node-token on stdin. Never enable shell tracing.
set -euo pipefail
umask 077

role=${1:?usage: bootstrap-k3s.sh server|agent name private-ip [server-ip] [management|target]}
name=${2:?node name required}
node_ip=${3:?private address required}
server_ip=${4:-}
plane=${5:-target}
version=v1.36.5+k3s1
case "$role:$plane" in server:management|server:target|agent:target) ;; *) exit 2 ;; esac
[[ $EUID == 0 ]] || { printf 'Run as root.\n' >&2; exit 2; }
[[ $name =~ ^hnb-[a-z-]+$ && $node_ip =~ ^10\.9\.[0-9]+\.[0-9]+$ ]] || exit 2
if command -v k3s >/dev/null; then
  printf 'K3s already exists; inspect the current cluster instead of overwriting it.\n' >&2
  exit 2
fi
available=$(df -B1 --output=avail / | tr -dc '0-9')
(( available >= 12 * 1024 * 1024 * 1024 )) || { printf 'Need 12GiB free before lab bootstrap.\n' >&2; exit 2; }
if [[ $role == agent ]]; then
  [[ $server_ip =~ ^10\.9\.[0-9]+\.[0-9]+$ ]] || exit 2
  IFS= read -r token
  [[ $token =~ ^[A-Za-z0-9:_-]+$ ]] || exit 2
fi

cache=/var/lib/hnb-lab/bootstrap
install -d -m 0700 "$cache" /etc/rancher/k3s
base="https://github.com/k3s-io/k3s/releases/download/${version/+/%2B}"
for asset in sha256sum-amd64.txt k3s k3s-airgap-images-amd64.tar.zst; do
  curl -fL --retry 3 --connect-timeout 20 --max-time 900 --silent --show-error \
    "$base/$asset" -o "$cache/$asset"
done
(cd "$cache" && sha256sum --check --ignore-missing sha256sum-amd64.txt)
curl -fL --retry 3 --max-time 60 --silent --show-error https://get.k3s.io -o "$cache/install.sh"
printf '%s  %s\n' e5cc3b3d9dfc1662c2d9be6da5abc9a4cd317d6abc3a5ffc02e3dd3248207fee "$cache/install.sh" | sha256sum --check
install -m 0755 "$cache/k3s" /usr/local/bin/k3s
install -d -m 0700 /var/lib/rancher/k3s/agent/images
mv "$cache/k3s-airgap-images-amd64.tar.zst" /var/lib/rancher/k3s/agent/images/
rm "$cache/k3s"

config=/etc/rancher/k3s/config.yaml
printf '# HNB dedicated S2/S3 lab\nnode-name: %s\nnode-ip: %s\n' "$name" "$node_ip" > "$config"
printf 'kubelet-arg:\n  - container-log-max-size=5Mi\n  - container-log-max-files=2\n  - image-gc-high-threshold=75\n  - image-gc-low-threshold=65\n  - max-pods=60\n' >> "$config"
if [[ $role == server ]]; then
  if [[ $plane == management ]]; then pod_cidr=10.42.0.0/16; service_cidr=10.43.0.0/16;
  else pod_cidr=10.44.0.0/16; service_cidr=10.45.0.0/16; fi
  printf 'bind-address: %s\nadvertise-address: %s\ncluster-cidr: %s\nservice-cidr: %s\ndisable:\n  - traefik\nwrite-kubeconfig-mode: "0600"\n' \
    "$node_ip" "$node_ip" "$pod_cidr" "$service_cidr" >> "$config"
else
  printf 'server: https://%s:6443\ntoken: %s\n' "$server_ip" "$token" >> "$config"
  unset token
fi
INSTALL_K3S_SKIP_DOWNLOAD=true INSTALL_K3S_SKIP_START=true INSTALL_K3S_EXEC="$role" sh "$cache/install.sh"
systemctl start "k3s$([[ $role == agent ]] && printf -- '-agent' || true)"
printf 'Started %s (%s), version %s. Verify nodes and system Pods separately.\n' "$name" "$role" "$version"
