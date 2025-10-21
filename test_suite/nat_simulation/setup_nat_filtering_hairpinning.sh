#!/usr/bin/env bash

pub_nat_iface=$1
priv_nat_iface=$2
priv_subnet=$3
nat_filter=$4

# Make sure all arguments have been passed, and nat_filter is between 0 and 2
if [[ $# -ne 4 || ! ($nat_filter =~ ^[0-2]$) ]]; then
    echo """
Usage: ${0} <PUBLIC NAT NETWORK INTERFACE> <PRIVATE NAT NETWORK INTERFACE> <PRIVATE SUBNET> <NAT FILTERING TYPE>

<NAT FILTERING TYPE> may be one of the following numbers:
    0 - Endpoint-Independent
    1 - Address-Dependent
    2 - Address and Port-Dependent

This script must be run with root permissions, and assumes the nftables postrouting chain already exists in the nat table"""
    exit 1
fi

# Configure NAT filtering type with nftables 
nft add table inet filter
nft add chain inet filter input { type filter hook input priority 0\; policy drop\; }
nft add rule inet filter input ct state related,established counter accept # This rule is sufficient to simulate APDF

# This pattern captures the following info from a conntrack event:
#   1) the source IP
#   2) the source port
#   3) the destination IP,
#   4) the translated source IP,
#   5) the translated source port.
pattern=".*src=(\S+).*sport=(\S+).*src=(\S+).*dst=(\S+).*dport=(\S+).*$"

# Hairpinning: if a mapping is created where source IP \1 and source port \2 are respectively translated to \4 and \5:
hairpin_rule1="nat prerouting iif $priv_nat_iface ip saddr $priv_subnet ip daddr \4 meta l4proto {tcp, udp} th dport \5 counter dnat to \1:\2" # All traffic from the private network destined to \4:\5 should be hairpinned pack to \1:\2
hairpin_rule2="nat postrouting iif $priv_nat_iface ip saddr \1 ip daddr $priv_subnet meta l4proto {tcp, udp} th sport \2 counter snat to \4:\5" # For all hairpinned packets from \1:\2, the source becomes \4:\5

# Filtering (not necessary for APDF because of filter rule above)
case $nat_filter in
    0)
        # If a mapping is created with source IP \1, source port \2 and translated source port \5, all traffic destined to \5 should be DNATed to \1:\2
        filter_rule="nat prerouting iif $pub_nat_iface meta l4proto {tcp, udp} th dport \5 counter dnat to \1:\2";;
    1)
        # If a mapping is created with source IP \1, source port \2, translated source IP \3 and translated source port \5, all traffic from \3 destined to \5 should be DNATed to \1:\2
        filter_rule="nat prerouting ip saddr \3 iif $pub_nat_iface meta l4proto {tcp, udp} th dport \5 counter dnat to \1:\2";;
esac

# Only monitor new source NAT connections that are created by the nftables NAT mapping rules
if [[ $nat_filter -eq 2 ]]; then
    conntrack -En -s $priv_subnet -e NEW | sed -rn -e "s#$pattern#nft add rule $hairpin_rule1; nft add rule $hairpin_rule2#e" # No filter rule for APDF NAT
else
    conntrack -En -s $priv_subnet -e NEW | sed -rn -e "s#$pattern#nft add rule $hairpin_rule1; nft add rule $hairpin_rule2; nft add rule $filter_rule#e"
fi