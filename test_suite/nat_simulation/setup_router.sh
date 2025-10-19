#!/usr/bin/env bash

router_name=$1
priv_name=$2
public_name=$3
priv_subnet=$4
priv_ip=$5
pub_subnet_prefix=$6
n_ips=$7
switch_ip=$8
router_index=$9

if [[ $# -ne 9 ]]; then
    echo """
Usage: ${0} <ROUTER NAME> <PRIVATE NETWORK NAME> <PUBLIC NETWORK NAME> <PRIVATE SUBNET> <ROUTER PRIVATE IP> <ROUTER PUBLIC IP> <NUMBER OF IPS> <SWITCH IP> <ROUTER INDEX>

This script must be run with root permissions"""
    exit 1
fi

pub_subnet="$pub_subnet_prefix.0/24"

# Create veth pair to place the router's private interface in the private and router namespaces
router_priv="${router_name}_priv"
ip link add $router_priv type veth peer $router_name netns $priv_name
ip netns exec $priv_name ip addr add "${priv_ip}/24" dev $router_name
ip link set $router_priv up
ip netns exec $priv_name ip link set $router_name up

# Add route for traffic to router's private network
ip route add $priv_ip dev $router_priv
ip route add $priv_subnet via $priv_ip dev $router_priv

# $router_index is equal to 0 for first router, 1 for additional router
if [[ $router_index -eq 0 ]]; then
    # Create veth pair to place the router's public interface in the public and router namespaces
    router_pub="${router_name}_pub"
    ip link add $router_pub type veth peer $router_name netns public

    for host in $(seq $((254 - $n_ips + 1)) 254); do
        ip="$pub_subnet_prefix.$host/24"
        ip addr add $ip dev $router_pub
    done

    ip link set $router_pub up
    ip netns exec public ip link set $router_name up

    # Create route to first router in the public network
    ip netns exec $public_name ip route add $pub_subnet dev $router_name
else
    # Veth pair already created by first router
    router_pub=$public_name
fi

# Show switch is routable via router as default gateway
ip route add $switch_ip dev $router_pub
ip route add default via $switch_ip dev $router_pub

