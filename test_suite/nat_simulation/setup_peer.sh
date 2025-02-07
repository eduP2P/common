#!/usr/bin/env bash

if [[ $# -ne 4 ]]; then
    echo """
Usage: ${0} <PEER NAME> <PRIVATE NETWORK NAME> <PEER IP> <ROUTER PRIVATE IP>

This script must be run with root permissions"""
    exit 1
fi

peer_name=$1
priv_name=$2
peer_ip=$3
router_ip=$4

# Create veth pair to give peer access to its private network
ip link add $peer_name type veth peer $peer_name netns $priv_name
ip link set $peer_name up
ip netns exec $priv_name ip link set $peer_name up

# Assign IP address
ip addr add $peer_ip dev $peer_name

# Add router as default gateway
ip route add $router_ip dev $peer_name
ip route add default via $router_ip dev $peer_name

# Add route to peer in private network
ip netns exec $priv_name ip route add $peer_ip dev $peer_name