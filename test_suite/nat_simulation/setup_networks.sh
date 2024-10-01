#!/bin/bash

if [[ $# -ne 2 ]]; then
    echo """
Usage: ${0} <NUMBER OF PRIVATE NETWORKS> <NUMBER OF PEERS PER PRIVATE NETWORK>

Simulates a network setup containing a specified number of private networks connected via the public network, with their routers applying Network Address Translation (NAT). The private networks contain a specified number of peers using eduP2P
This script must be run with root permissions"""
    exit 1
fi

n_priv_nets=$1
n_peers=$2

# Create namespace to simulate the public network
./create_namespace.sh public

# Setup public network
pub_prefix="192.168.0"
switch_ip="${pub_prefix}.254"
control_ip="${pub_prefix}.1"
relay_ip="${pub_prefix}.2"
ip netns exec public ./setup_public.sh $switch_ip $control_ip $relay_ip

# Setup routers and private networks
for ((i=1; i<=n_priv_nets; i++)); do
    # Create namespace for the router
    router_name="router${i}"
    ./create_namespace.sh $router_name

    # Create namespace for the private network
    priv_name="private${i}"
    ./create_namespace.sh $priv_name
    
    # Variables related to the private network and its router
    priv_prefix="10.0.${i}"
    priv_subnet="${priv_prefix}.0/24"
    router_priv_ip="${priv_prefix}.254"
    pub_prefix="192.168.${i}"
    router_pub_ip="${pub_prefix}.254"

    # Setup router
    ip netns exec $router_name ./setup_router.sh $router_name $priv_name $priv_subnet $router_priv_ip $router_pub_ip $switch_ip 

    # Setup private network
    ip netns exec $priv_name ./setup_private.sh $router_name $router_pub_ip

    # Setup peers in each private network
    for ((j=1; j<=n_peers; j++)); do
        # Create namespace for the peer in the private network
        peer_name="${priv_name}_peer${j}"
        peer_ip="${priv_prefix}.${j}"
        ./create_namespace.sh $peer_name

        # Setup peer
        ip netns exec $peer_name ./setup_peer.sh $peer_name $priv_name $peer_ip $router_priv_ip 
    done
done