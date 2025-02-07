#!/usr/bin/env bash

if [[ $# -ne 3 ]]; then
    echo """
Usage: ${0} <ROUTER NAME> <ROUTER PUBLIC IP> <PRIVATE SUBNET>

This script must be run with root permissions"""
    exit 1
fi

router_name=$1
router_ip=$2
priv_subnet=$3

# Set router as default gateway
ip route add $router_ip dev $router_name
ip route add default via $router_ip dev $router_name

# Add nft rule to block traffic between hosts in the private network
nft add table inet filter
nft add chain inet filter forward { type filter hook forward priority 0\; policy accept\; }
nft add rule inet filter forward ip saddr $priv_subnet ip daddr $priv_subnet reject