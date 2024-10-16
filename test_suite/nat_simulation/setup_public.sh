#!/bin/bash

if [[ $# -ne 3 ]]; then
    echo """
Usage: ${0} <SWITCH IP> <CONTROL SERVER IP> <RELAY SERVER IP>

This script must be run with root permissions"""
    exit 1
fi

switch_ip=$1
control_ip=$2
relay_ip=$3

# Turn on loopback interface in namespace to allow pinging from inside namespace
ip link set lo up

# Create TUN interface for switch
ip tuntap add dev switch mode tun
ip link set dev switch up
ip addr add "${switch_ip}/24" dev switch

# Create TUN interface for control server
ip tuntap add dev control mode tun
ip link set dev control up
ip addr add "${control_ip}/24" dev control  

# Create TUN interface for relay server
ip tuntap add dev relay mode tun
ip link set dev relay up
ip addr add "${relay_ip}/24" dev relay