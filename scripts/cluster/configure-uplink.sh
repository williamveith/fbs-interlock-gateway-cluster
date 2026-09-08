#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <NetworkManager connection> <shared MAC>" >&2
    exit 2
fi

connection_name=$1
shared_mac=$2

if [[ $EUID -ne 0 ]]; then
    echo "ERROR: run this script as root." >&2
    exit 1
fi

command -v nmcli >/dev/null 2>&1 || {
    echo "ERROR: nmcli is required." >&2
    exit 1
}

nmcli connection show "$connection_name" >/dev/null

nmcli connection modify "$connection_name" \
    802-11-wireless.cloned-mac-address "$shared_mac" \
    connection.autoconnect no

# Fail closed. The controller will bring this profile up only on the node
# assigned the gateway task and still connected to a manager quorum peer.
nmcli connection down "$connection_name" >/dev/null 2>&1 || true

echo "Configured '$connection_name' with shared MAC $shared_mac."
echo "Autoconnect is disabled; fbs-swarm-uplink will control this connection."
