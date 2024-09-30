#!/bin/bash

# Usage: ./setup_private.sh <ROUTER NAME> <ROUTER PUBLIC IP>
# This script must be run with root permissions

router_name=$1
router_ip=$2

# Turn on loopback interface in namespace to allow pinging from inside namespace
ip link set lo up

# Set router as default gateway
ip route add $router_ip dev $router_name
ip route add default via $router_ip dev $router_name

