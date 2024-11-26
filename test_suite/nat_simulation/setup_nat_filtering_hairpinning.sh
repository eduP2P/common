#!/bin/bash

pub_nat_iface=$1
priv_nat_iface=$2
pub_ip=$3
priv_subnet=$4
nat_filter=$5

# Make sure all arguments have been passed, and nat_filter is between 0 and 2
if [[ $# -ne 5 || ! ($nat_filter =~ ^[0-2]$) ]]; then
    echo """
Usage: ${0} <PUBLIC NAT NETWORK INTERFACE> <PRIVATE NAT NETWORK INTERFACE> <PUBLIC IP> <PRIVATE SUBNET> <NAT FILTERING TYPE>

<NAT FILTERING TYPE> may be one of the following numbers:
0 - Endpoint-Independent
1 - Address-Dependent
2 - Address and Port-Dependent

This script must be run with root permissions, and assumes the nftables postrouting chain already exists in the nat table"""
    exit 1
fi

# Configure NAT filtering type with nftables 
nft add chain nat prerouting { type nat hook prerouting priority -100\; }

nft add table inet filter
nft add chain inet filter input { type filter hook input priority 0\; policy drop\; }
nft add rule inet filter input ct state related,established counter accept

# This pattern captures 1) the source IP, 2) the source port, 3) the destination IP, and 4) the translated source port from an event
pattern=".*src=(\S+).*sport=(\S+).*src=(\S+).*dport=(\S+)$"

# Hairpinning: if a mapping is created with source IP \1, source port \2 and translated source port \4: 
hairpin_rule1="nat prerouting iif $priv_nat_iface ip saddr $priv_subnet ip daddr $pub_ip meta l4proto {tcp, udp} th dport \4 counter dnat to \1:\2" # All traffic from the private network destined to the router's public IP should be hairpinned pack to \1:\2
hairpin_rule2="nat postrouting iif $priv_nat_iface ip saddr \1 ip daddr $priv_subnet meta l4proto {tcp, udp} th sport \2 counter snat to $pub_ip:\4" # Fpr a;; traffic that is hairpinned by the previous rule, the source of the hairpinned packets becomes the external source IP and port

# Filtering (not necessary for ADPF)
case $nat_filter in
    0)
    # If a mapping is created with source IP \1, source port \2 and translated source port \4, all traffic destined to \4 should be forwarded to \1:\2
    filter_rule="nat prerouting iif $pub_nat_iface meta l4proto {tcp, udp} th dport \4 counter dnat to \1:\2";;
    1)
    # If a mapping is created with source IP \1, source port \2, translated source IP \3 and translated source port \4, all traffic from \3 destined to \4 should be forwarded to \1:\2
    filter_rule="nat prerouting ip saddr \3 iif $pub_nat_iface meta l4proto {tcp, udp} th dport \4 counter dnat to \1:\2";;
esac

# Only monitor new source NAT connections that are created by the nftables masquerade rule
if [[ $nat_filter -eq 2 ]]; then
    conntrack -En -s $priv_subnet -e NEW | sed -rn -e "s#$pattern#nft add rule $hairpin_rule1; nft add rule $hairpin_rule2#e"
else
    conntrack -En -s $priv_subnet -e NEW | sed -rn -e "s#$pattern#nft add rule $hairpin_rule1; nft add rule $hairpin_rule2; nft add rule $filter_rule#e"
fi