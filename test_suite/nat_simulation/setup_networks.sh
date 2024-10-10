#!/bin/bash

if [[ $1 = "help" ]]; then
    echo """
Usage: ${0} 

Simulates a network setup containing two private networks connected via the public network, with their routers applying different forms of Network Address Translation (NAT). Each private network contains two peers using eduP2P
This script must be run with root permissions"""
    exit 1
fi

# Create namespace to simulate the public network
./create_namespace.sh public

# Setup public network
pub_prefix="192.168.0"
switch_ip="${pub_prefix}.254"
control_ip="${pub_prefix}.1"
relay_ip="${pub_prefix}.2"
ip netns exec public ./setup_public.sh $switch_ip $control_ip $relay_ip

# Keep track of a list of IP addresses that each peer may communicate with to simulate NAT with an Address-Dependent Mapping
adm_ips="adm_ips.txt"
touch $adm_ips

# Add control and relay server to the list
echo $control_ip >> $adm_ips
echo $relay_ip >> $adm_ips

# Two private networks that will have different NAT behaviour depending on the system test's parameters
n_priv_nets=2

# Two peers per network to test NAT hairpinning
n_peers=2

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

    # Add router's public IP to list created earlier
    echo $router_pub_ip >> $adm_ips

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

# Setup NAT in the routers
# ip netns exec router1 ./setup_nat.sh router1_pub 10.0.1.0/24 0 0 $adm_ips
# ip netns exec router2 ./setup_nat.sh router2_pub 10.0.2.0/24 0 1 $adm_ips
# ip netns exec router3 ./setup_nat.sh router3_pub 10.0.3.0/24 0 2 $adm_ips

# ip netns exec router4 ./setup_nat.sh router4_pub 10.0.4.0/24 1 0 $adm_ips
# ip netns exec router5 ./setup_nat.sh router5_pub 10.0.5.0/24 1 1 $adm_ips
# ip netns exec router6 ./setup_nat.sh router6_pub 10.0.6.0/24 1 2 $adm_ips

# ip netns exec router7 ./setup_nat.sh router7_pub 10.0.7.0/24 2 0 $adm_ips
# ip netns exec router8 ./setup_nat.sh router8_pub 10.0.8.0/24 2 1 $adm_ips
# ip netns exec router9 ./setup_nat.sh router9_pub 10.0.9.0/24 2 2 $adm_ips

