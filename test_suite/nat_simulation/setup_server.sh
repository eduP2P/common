#!/bin/bash

if [[ $# -ne 3 ]]; then
    echo """
Usage: ${0} <SWITCH IP> <SERVER NAMESPACE> <SERVER IP>

This script must be run with root permissions"""
    exit 1
fi

switch_ip=$1
server_ns=$2
server_ip=$3

# Create veth pair for server
ip link add $server_ns type veth peer $server_ns netns public
ip link set dev $server_ns up
ip addr add "${server_ip}/32" dev $server_ns   
ip netns exec public ip link set $server_ns up

# Make switch the default gateway
ip route add $switch_ip dev $server_ns
ip route add default via $switch_ip

# Create route to server in the public network
ip netns exec public ip route add $server_ip dev $server_ns

