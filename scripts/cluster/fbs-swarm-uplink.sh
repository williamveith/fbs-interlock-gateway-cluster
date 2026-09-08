#!/bin/bash
set -u

config_file=${FBS_SWARM_UPLINK_CONFIG:-/etc/default/fbs-swarm-uplink}

if [[ ! -r $config_file ]]; then
    echo "ERROR: cannot read $config_file" >&2
    exit 1
fi

# shellcheck source=/dev/null
source "$config_file"

: "${UPLINK_CONNECTION:?UPLINK_CONNECTION is required}"
: "${SWARM_SERVICE:?SWARM_SERVICE is required}"
: "${PEER_MANAGER_IPS:?PEER_MANAGER_IPS is required}"
: "${CHECK_INTERVAL:=1}"

log() {
    printf '%s %s\n' "$(date --iso-8601=seconds)" "$*"
}

uplink_is_active() {
    nmcli -t -f NAME connection show --active 2>/dev/null | \
        grep -Fxq "$UPLINK_CONNECTION"
}

uplink_up() {
    if uplink_is_active; then
        return 0
    fi

    if nmcli connection up "$UPLINK_CONNECTION" >/dev/null; then
        log "enabled uplink '$UPLINK_CONNECTION'"
        return 0
    fi

    log "failed to enable uplink '$UPLINK_CONNECTION'"
    return 1
}

uplink_down() {
    if ! uplink_is_active; then
        return 0
    fi

    if nmcli connection down "$UPLINK_CONNECTION" >/dev/null 2>&1; then
        log "disabled uplink '$UPLINK_CONNECTION'"
        return 0
    fi

    log "failed to disable uplink '$UPLINK_CONNECTION'"
    return 1
}

node_is_ready_manager() {
    local state
    state=$(docker node inspect --format '{{.Status.State}}' self 2>/dev/null) || return 1
    [[ $state == ready ]]
}

gateway_task_assigned_here() {
    docker node ps \
        --filter "name=$SWARM_SERVICE" \
        --filter 'desired-state=running' \
        --format '{{.Name}}' \
        2>/dev/null | grep -q .
}

peer_manager_reachable() {
    local peer

    for peer in $PEER_MANAGER_IPS; do
        if timeout 1 bash -c "exec 3<>/dev/tcp/$peer/2377" \
            >/dev/null 2>&1; then
            return 0
        fi
    done

    return 1
}

should_own_uplink() {
    node_is_ready_manager && \
        gateway_task_assigned_here && \
        peer_manager_reachable
}

fail_closed() {
    uplink_down || true
}

trap fail_closed EXIT INT TERM

log "starting for service '$SWARM_SERVICE' on '$UPLINK_CONNECTION'"

while true; do
    if should_own_uplink; then
        uplink_up || true
    else
        uplink_down || true
    fi

    sleep "$CHECK_INTERVAL"
done
