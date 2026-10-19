#!/usr/bin/env bash
# Test-only, fully allocated backing on the root filesystem; never a real disk PV.
set -euo pipefail
umask 077
size=${1:?usage: prepare-loop-storage.sh 3|5}
[[ $EUID == 0 && $size =~ ^[35]$ ]] || exit 2
path=/var/lib/hnb-lab/storage/backing.img
vg=hnb-lab-vg
if [[ -e $path ]] || vgs "$vg" >/dev/null 2>&1; then
  printf 'Lab storage already exists; refusing to replace it.\n' >&2
  exit 2
fi
available=$(df -B1 --output=avail / | tr -dc '0-9')
(( available >= (size + 7) * 1024 * 1024 * 1024 )) || { printf 'Insufficient disk reserve.\n' >&2; exit 2; }
install -d -m 0700 "$(dirname "$path")"
fallocate -l "${size}G" "$path"
device=$(losetup --find --show "$path")
pvcreate "$device"
vgcreate "$vg" "$device"
cat > /etc/systemd/system/hnb-lab-loop.service <<'UNIT'
[Unit]
Description=HNB lab loop-backed CSI storage (development only)
Before=k3s.service k3s-agent.service
After=local-fs.target
RequiresMountsFor=/var/lib/hnb-lab/storage

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -ec '/usr/sbin/losetup -j /var/lib/hnb-lab/storage/backing.img | /usr/bin/grep -q . || /usr/sbin/losetup --find /var/lib/hnb-lab/storage/backing.img; /usr/sbin/vgchange -ay hnb-lab-vg'

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now hnb-lab-loop.service
vgs "$vg"
df -h /
