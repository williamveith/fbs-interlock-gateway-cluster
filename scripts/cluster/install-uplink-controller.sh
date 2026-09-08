#!/bin/bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
    echo "Usage: $0 <uplink connection> <swarm service> <peer manager IPs> <check interval>" >&2
    exit 2
fi

uplink_connection=$1
swarm_service=$2
peer_manager_ips=$3
check_interval=$4

if [[ $EUID -ne 0 ]]; then
    echo "ERROR: run this script as root." >&2
    exit 1
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/../.." && pwd)

install -m 0755 \
    "$repo_root/scripts/cluster/fbs-swarm-uplink.sh" \
    /usr/local/sbin/fbs-swarm-uplink

install -m 0644 \
    "$repo_root/services/linux/fbs-swarm-uplink.service" \
    /etc/systemd/system/fbs-swarm-uplink.service

{
    printf 'UPLINK_CONNECTION=%q\n' "$uplink_connection"
    printf 'SWARM_SERVICE=%q\n' "$swarm_service"
    printf 'PEER_MANAGER_IPS=%q\n' "$peer_manager_ips"
    printf 'CHECK_INTERVAL=%q\n' "$check_interval"
} > /etc/default/fbs-swarm-uplink

chmod 0600 /etc/default/fbs-swarm-uplink

systemctl daemon-reload
systemctl enable --now fbs-swarm-uplink.service

echo "Installed and started fbs-swarm-uplink.service."
