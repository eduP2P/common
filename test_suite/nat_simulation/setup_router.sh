#!/usr/bin/env bash

router_name=$1
priv_name=$2
priv_subnet=$3
priv_ip=$4
pub_subnet_prefix=$5
n_ips=$6
switch_ip=$7

if [[ $# -ne 7 ]]; then
    echo """
Usage: ${0} <ROUTER NAME> <PRIVATE NETWORK NAME> <PRIVATE SUBNET> <ROUTER PRIVATE IP> <PUBLIC /24 NETWORK PREFIX> <NUMBER OF IPS>  <SWITCH IP>

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

# Create veth pair to place the router's public interface in the public and router namespaces
router_pub="${router_name}_pub"
ip link add $router_pub type veth peer $router_name netns public

for host in $(seq $((254 - $n_ips + 1)) 254); do
    ip="$pub_subnet_prefix.$host/24"
    ip addr add $ip dev $router_pub
done

ip link set $router_pub up
ip netns exec public ip link set $router_name up

# Add switch as default gateway
ip route add $switch_ip dev $router_pub
ip route add default via $switch_ip dev $router_pub

# Add route for traffic to router's private network
ip route add $priv_ip dev $router_priv
ip route add $priv_subnet via $priv_ip dev $router_priv

# Create route to router in the public network
ip netns exec public ip route add $pub_subnet dev $router_name