#!/bin/bash

router_name=$1
priv_name=$2
priv_subnet=$3
priv_ip=$4
pub_ip=$5
switch_ip=$6

if [[ $# -ne 6 ]]; then
    echo """
Usage: ${0} <ROUTER NAME> <PRIVATE NETWORK NAME> <PRIVATE SUBNET> <ROUTER PRIVATE IP> <ROUTER PUBLIC IP> <SWITCH IP>

This script must be run with root permissions"""
    exit 1
fi

# Create veth pair to place the router's private interface in the private and router namespaces
router_priv="${router_name}_priv"
ip link add $router_priv type veth peer $router_name netns $priv_name
ip netns exec $priv_name ip addr add "${priv_ip}/24" dev $router_name
ip link set $router_priv up
ip netns exec $priv_name ip link set $router_name up

# Create veth pair to place the router's public interface in the public and router namespaces
router_pub="${router_name}_pub"
ip link add $router_pub type veth peer $router_name netns public
ip addr add "${pub_ip}/24" dev $router_pub
ip link set $router_pub up
ip netns exec public ip link set $router_name up

# Add switch as default gateway
ip route add $switch_ip dev $router_pub
ip route add default via $switch_ip dev $router_pub

# Add route for traffic to router's private network
ip route add $priv_ip dev $router_priv
ip route add $priv_subnet via $priv_ip dev $router_priv

# Create route to router in the public network
ip netns exec public ip route add $pub_ip dev $router_name