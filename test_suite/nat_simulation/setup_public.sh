#!/usr/bin/env bash

if [[ $# -ne 1 ]]; then
    echo """
Usage: ${0} <SWITCH IP>

This script must be run with root permissions"""
    exit 1
fi

switch_ip=$1

# Create TUN interface for switch
ip tuntap add dev switch mode tun
ip link set dev switch up
ip addr add "${switch_ip}/24" dev switch

# Create nftables configuration to simulate packet loss
nft add table inet filter
nft add chain inet filter forward { type filter hook forward priority 0\; policy accept\; }