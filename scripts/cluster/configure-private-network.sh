#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <NetworkManager connection> <IPv4 CIDR>" >&2
    echo "Example: $0 'Wired connection 1' 10.50.0.11/24" >&2
    exit 2
fi

connection_name=$1
node_address=$2

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
    ipv4.method manual \
    ipv4.addresses "$node_address" \
    ipv4.gateway "" \
    ipv4.dns "" \
    ipv4.never-default yes \
    ipv6.method disabled \
    connection.autoconnect yes

nmcli connection up "$connection_name"

echo "Configured private cluster connection '$connection_name' as $node_address."
echo "No default gateway or DNS is configured on this connection."
