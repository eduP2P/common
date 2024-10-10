#!/bin/bash

nat_iface=$1
nat_filter=$2

# Make sure all arguments have been passed, and nat_filter is between 0 and 2
if [[ $# -ne 2 || ! ($nat_filter =~ ^[0-2]$)]]; then
    echo ="""
Usage: ${0} <ROUTER NAME> <NAT NETWORK INTERFACE> <NAT FILTERING TYPE>

<NAT FILTERING TYPE> may be one of the following numbers:
0 - Endpoint-Independent
1 - Address-Dependent
2 - Address and Port-Dependent

This script must be run with root permissions"""
    exit 1
fi

# This pattern captures 1) the internal source IP, 2) destination IP, and 3) external source port from an event
pattern=".*src=(\S+).*src=(\S+).*dport=(\S+)$"

case $nat_filter in
    0)
    # If a mapping is created with internal source IP \1 and external source port \3, all traffic destined to \3 should be forwarded to \1
    nft_rule="nat prerouting iif $nat_iface meta l4proto {tcp, udp} th dport \3 dnat to \1";;
    1)
    # If a mapping is created with internal source IP \1 and external source port \3 for a packet with destination IP \2, all traffic from \2 destined to \3 should be forwarded to \1
    nft_rule="nat prerouting ip saddr \2 iif $nat_iface meta l4proto {tcp, udp} th dport \3 dnat to \1";;
    2)
    # Monitoring conntrack is not necessary for APDF
    exit 0;;
esac

# Only monitor new source NAT connections that are created by the nftables masquerade rule
conntrack -En -e NEW | sed -rn -e "s/$pattern/nft add rule $nft_rule/e"