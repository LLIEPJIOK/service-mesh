#!/bin/sh
set -e

: "${INBOUND_PORTS:?INBOUND_PORTS is required}"
: "${INBOUND_PLAIN_PORT:?INBOUND_PLAIN_PORT is required}"
: "${OUTBOUND_PORT:?OUTBOUND_PORT is required}"
: "${UID:?UID is required}"

SIDECAR_UID="$UID"

split_csv() {
    echo "$1" | tr ',' '\n' | while IFS= read -r item; do
        trimmed=$(echo "$item" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
        if [ -n "$trimmed" ]; then
            echo "$trimmed"
        fi
    done
}

cleanup_jump_rules() {
    source_chain="$1"
    target_chain="$2"

    iptables -t nat -S "$source_chain" 2>/dev/null | while IFS= read -r rule; do
        if echo "$rule" | grep -Eq "^-A[[:space:]]+$source_chain([[:space:]]|$)" \
            && echo "$rule" | grep -Eq "[[:space:]]-j[[:space:]]+$target_chain([[:space:]]|$)"; then
            delete_rule="${rule/-A /-D }"
            iptables -t nat $delete_rule || true
        fi
    done
}

cleanup_jump_rules PREROUTING MESH_INBOUND
cleanup_jump_rules OUTPUT MESH_OUTPUT

iptables -t nat -F MESH_INBOUND 2>/dev/null || true
iptables -t nat -X MESH_INBOUND 2>/dev/null || true
iptables -t nat -F MESH_OUTPUT 2>/dev/null || true
iptables -t nat -X MESH_OUTPUT 2>/dev/null || true

iptables -t nat -N MESH_INBOUND
iptables -t nat -N MESH_OUTPUT

for port in $(split_csv "$INBOUND_PORTS"); do
    iptables -t nat -A PREROUTING -p tcp --dport "$port" -j MESH_INBOUND
done

if [ -n "$EXCLUDE_INBOUND_PORTS" ]; then
    for port in $(split_csv "$EXCLUDE_INBOUND_PORTS"); do
        iptables -t nat -A MESH_INBOUND -p tcp --dport "$port" -j RETURN
    done
fi

iptables -t nat -A MESH_INBOUND -p tcp -j REDIRECT --to-port "$INBOUND_PLAIN_PORT"

iptables -t nat -A OUTPUT -j MESH_OUTPUT
iptables -t nat -A MESH_OUTPUT -m owner --uid-owner "$SIDECAR_UID" -j RETURN

if [ -n "$EXCLUDE_OUTBOUND_IPS" ]; then
    for ip in $(split_csv "$EXCLUDE_OUTBOUND_IPS"); do
        iptables -t nat -A MESH_OUTPUT -d "$ip" -j RETURN
    done
fi

iptables -t nat -A MESH_OUTPUT -d 127.0.0.0/8 -j RETURN
iptables -t nat -A MESH_OUTPUT -p udp --dport 53 -j RETURN
iptables -t nat -A MESH_OUTPUT -p tcp --dport 53 -j RETURN
iptables -t nat -A MESH_OUTPUT -p tcp -j REDIRECT --to-port "$OUTBOUND_PORT"

echo "iptables rules applied successfully"