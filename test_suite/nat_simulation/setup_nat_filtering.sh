#!/bin/bash

nat_iface=$1
priv_subnet=$2
nat_filter=$3

# Make sure all arguments have been passed, and nat_filter is between 0 and 2
if [[ $# -ne 3 || ! ($nat_filter =~ ^[0-2]$) ]]; then
    echo ="""
Usage: ${0} <NAT NETWORK INTERFACE> <PRIVATE SUBNET> <NAT FILTERING TYPE>

<NAT FILTERING TYPE> may be one of the following numbers:
0 - Endpoint-Independent
1 - Address-Dependent
2 - Address and Port-Dependent

This script must be run with root permissions"""
    exit 1
fi

# Configure NAT filtering type with nftables 
nft add chain nat prerouting { type nat hook prerouting priority -100\; }

nft add table inet filter
nft add chain inet filter input { type filter hook input priority 0\; policy drop\; }
nft add rule inet filter input ct state related,established counter accept
nft add rule inet filter input ct state new counter reject

# This pattern captures 1) the internal source IP, 2) the internal source port, 3) the destination IP, and 4) the external source port from an event
pattern=".*src=(\S+).*sport=(\S+).*src=(\S+).*dport=(\S+)$"

case $nat_filter in
    0)
    # If a mapping is created with internal source IP \1, internal source port \2 and external source port \4, all traffic destined to \4 should be forwarded to \1:\2
    nft_rule="nat prerouting iif $nat_iface meta l4proto {tcp, udp} th dport \4 counter dnat to \1:\2";;
    1)
    # If a mapping is created with internal source IP \1 and external source port \3 for a packet with destination IP \2, all traffic from \2 destined to \3 should be forwarded to \1
    # If a mapping is created with internal source IP \1, internal source port \2 and external source port \4, all traffic from \3 destined to \4 should be forwarded to \1:\2
    nft_rule="nat prerouting ip saddr \\3 iif $nat_iface meta l4proto {tcp, udp} th dport \4 counter dnat to \1:\2";;
    2)
    # Monitoring conntrack is not necessary for APDF
    exit 0;;
esac

# Only monitor new source NAT connections that are created by the nftables masquerade rule
conntrack -En -s $priv_subnet -e NEW | sed -rn -e "s/$pattern/nft add rule $nft_rule/e"