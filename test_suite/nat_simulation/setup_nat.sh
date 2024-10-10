#!/bin/bash

nat_iface=$1
priv_subnet=$2
nat_map=$3
nat_filter=$4
adm_ips=$5

# Make sure all arguments have been passed, and nat_map and nat_filter are both integers between 0 and 2
if [[ $# -ne 5 || ! ($nat_map =~ ^[0-2]+$ && $nat_filter =~ ^[0-2]+$) ]]; then
    echo """
Usage: ${0} <NAT NETWORK INTERFACE> <PRIVATE SUBNET> <NAT MAPPING TYPE> <NAT FILTERING TYPE> <IP ADDRESS LIST>

<NAT MAPPING TYPE> and <NAT FILTERING TYPE> may be one of the following numbers:
0 - Endpoint-Independent
1 - Address-Dependent
2 - Address and Port-Dependent

<IP ADDRESS LIST> contains IP addresses that may be the destination IP of packets crossing this NAT device, and are necessary to simulate an Address-Dependent Mapping

This script must be run with root permissions"""
    exit 1
fi


# Configure NAT mapping type with nftables rules
nft add table ip nat
nft add chain ip nat postrouting { type nat hook postrouting priority 100\; }

case $nat_map in
    0) 
    nft add rule ip nat postrouting ip saddr $priv_subnet oif $nat_iface counter masquerade persistent;;
    1) 
    # Assign a block of 100 ports to each IP in adm_ips
    port_range_start=50000
    port_range_width=100

    # Iterate over each IP
    while read line; do
        ip=$line

        port_range_end=$(($port_range_start + $port_range_width - 1))
        nft add rule ip nat postrouting ip protocol {tcp, udp} ip saddr $priv_subnet ip daddr $ip oif $nat_iface counter masquerade to :${port_range_start}-${port_range_end} persistent
        port_range_start=$(($port_range_end+1))
    done < $adm_ips;;
    2) 
    nft add rule ip nat postrouting ip saddr $priv_subnet oif $nat_iface counter masquerade random;;
esac

# Configure NAT filtering type with nftables 
nft add chain nat prerouting { type nat hook prerouting priority -100\; }

nft add table inet filter
nft add chain inet filter input { type filter hook input priority 0\; policy drop\; }
nft add rule inet filter input ct state related,established accept

echo "Initial nftables configuration:"
nft list ruleset

function cleanup() {
    echo "Final nftables configuration:"
    nft list ruleset
}

trap cleanup EXIT

# For some NAT filtering behaviour, it is necessary to dynamically add rules based on monitoring conntrack events
./monitor_conntrack.sh $nat_iface $nat_filter