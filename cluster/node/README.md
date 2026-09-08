# Physical Debian Swarm node networking

This directory covers host-level networking for the three-node Debian 13
(Trixie) gateway cluster. `cluster/swarm/stack.yml` remains responsible for
the gateway service itself; physical Ethernet/Wi-Fi configuration and Swarm
membership remain host responsibilities.

## Network model

Each physical node has two independent network roles:

- **Private Ethernet:** unique static address per node. Docker Swarm control,
  Raft, overlay/data-path traffic, and cross-node routing use this network.
- **`utexas-iot` Wi-Fi:** all three profiles are configured with the same
  cloned MAC, but only one profile is allowed to be active at a time.

The Wi-Fi owner follows the node assigned the `fbs_gateway` task. The
host-side controller enables `utexas-iot` only when all of these are true:

1. the local node is a ready Swarm manager;
2. the local node has the gateway task in desired state `Running`; and
3. at least one other manager is reachable over the private network on TCP
   2377.

If any condition fails, the Wi-Fi profile is disabled. This third condition
provides fencing during a private-network partition so an isolated old task
does not keep the shared MAC while the remaining two-manager quorum
reschedules the gateway.

## Example private addresses

These are examples only; use the addresses selected for the actual cluster.

| Node | Private address | Peer manager addresses |
| --- | --- | --- |
| node1 | `10.50.0.11/24` | `10.50.0.12 10.50.0.13` |
| node2 | `10.50.0.12/24` | `10.50.0.11 10.50.0.13` |
| node3 | `10.50.0.13/24` | `10.50.0.11 10.50.0.12` |

The private Ethernet profile is configured with no default gateway and no
DNS so it cannot replace the normal `utexas-iot` route.

## 1. Configure private Ethernet on every node

First identify the NetworkManager connection name:

```bash
nmcli connection show
```

Example for node1:

```bash
make node-private-network \
  PRIVATE_CONNECTION="Wired connection 1" \
  NODE_ADDRESS=10.50.0.11/24
```

Repeat on node2 and node3 with their addresses.

Verify direct private connectivity before creating the Swarm:

```bash
ping 10.50.0.12
ping 10.50.0.13
```

## 2. Initialize the first manager

On node1:

```bash
make swarm-init-private NODE_IP=10.50.0.11
```

The target pins Swarm manager and data-path traffic to the private address.

Get the manager join token:

```bash
make swarm-manager-token
```

Store the token in a root-readable temporary file if you do not want it in
shell history:

```bash
sudo sh -c 'umask 077; cat > /root/fbs-swarm-manager.token'
```

Paste the token, then press `Ctrl-D`.

## 3. Join node2 and node3 as managers

On node2:

```bash
make swarm-join-manager \
  NODE_IP=10.50.0.12 \
  MANAGER_IP=10.50.0.11 \
  MANAGER_TOKEN_FILE=/root/fbs-swarm-manager.token
```

On node3:

```bash
make swarm-join-manager \
  NODE_IP=10.50.0.13 \
  MANAGER_IP=10.50.0.11 \
  MANAGER_TOKEN_FILE=/root/fbs-swarm-manager.token
```

From any manager, verify three managers are present:

```bash
docker node ls
```

## 4. Pre-create the gateway data volume on every node

The stack uses an external local volume, so run this once on **each** node:

```bash
make swarm-volume
```

This also sets `/data` ownership for UID/GID `65532`, which is the gateway
container user.

## 5. Configure and install shared-MAC uplink control

Do this only after the three-manager Swarm is healthy. The command disables
normal autoconnect for the Wi-Fi profile and hands control to the systemd
service.

Example node1:

```bash
make swarm-uplink-install \
  UPLINK_CONNECTION=utexas-iot \
  SHARED_MAC=02:00:00:00:00:11 \
  PEER_MANAGER_IPS="10.50.0.12 10.50.0.13"
```

Node2 uses peers `10.50.0.11 10.50.0.13`; node3 uses peers
`10.50.0.11 10.50.0.12`. Use the actual shared MAC selected for the cluster.

Check the controller:

```bash
make swarm-uplink-status UPLINK_CONNECTION=utexas-iot
journalctl -u fbs-swarm-uplink.service -f
```

Before a gateway task exists, all three `utexas-iot` profiles should remain
down. When Swarm assigns `fbs_gateway` to one node, that node should activate
the shared-MAC Wi-Fi profile and the other two should remain down.

## 6. Deploy the gateway

The existing `cluster/swarm/stack.yml` does not need host-network fields. Once
all nodes have the image available and the external volume has been prepared
on each node, deploy the stack from a manager using the existing deployment
target.

## Failure behavior

If the gateway node disappears, the remaining two managers retain quorum,
Swarm assigns the gateway task to another node, and that node's uplink
controller activates the same `utexas-iot` MAC. Litestream can then restore
or catch up the local SQLite database from R2 before the gateway resumes
normal operation.

If the old gateway node is merely isolated from the private Ethernet instead
of losing power, it cannot reach either peer manager on TCP 2377, so its
controller disables `utexas-iot` before a replacement node assumes the shared
MAC.
