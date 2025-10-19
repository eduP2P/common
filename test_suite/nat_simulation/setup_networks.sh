#!/usr/bin/env bash

usage_str="""Usage: ${0} [OPTIONAL ARGUMENTS]

Simulates a network setup containing two private networks connected via the public network. Each private network contains two peers using eduP2P

By default, a single NAT with one IP address facilitates the communication between a private and public network. With the -2 flag, Double NAT is simulated by adding another level of private networks on top of the existing ones. With the -n flag, the amount of IPs on the NAT can be increased up to 9; if Double NAT is enabled, the additional NAT always has 1 IP address.

To allow traffic to flow between the public and private networks, the script setup_nat_mapping.sh should also be executed. To allow traffic to flow between peers in the same private network, the script setup_nat_filtering_hairpinning.sh should also be executed

This script must be run with root permissions"""

# Use functions and constants from util.sh
. ../util.sh

# Default arguments
n_pooling_ips=1

# Validate optional arguments
while getopts ":2n:h" opt; do
    case $opt in
        2)
            double_nat=true
            ;;
        n)
            n_pooling_ips=$OPTARG

            # Make sure n_pooling_ips is an integer between 1 and 9
            n_pooling_ips_regex="^[1-9]$"
            validate_str $n_pooling_ips $n_pooling_ips_regex
            ;;
        h) 
            echo "$usage_str"
            exit 0
            ;;
        *)
            exit_with_error "invalid option"
            ;;
    esac
done

# Enable IP forwarding to allow for routing between namespaces
sysctl -w net.ipv4.ip_forward=1 &> /dev/null

# Create namespace to simulate the public network
./create_namespace.sh public

# Setup public network
pub_prefix="192.168.0"
switch_ip="${pub_prefix}.254"
ip netns exec public ./setup_public.sh $switch_ip

# Create namespaces to simulate the eduP2P control and relay server
./create_namespace.sh control
./create_namespace.sh relay

# Setup servers
control_ip="${pub_prefix}.1"
relay_ip="${pub_prefix}.2"
ip netns exec control ./setup_server.sh $switch_ip control $control_ip
ip netns exec relay ./setup_server.sh $switch_ip relay $relay_ip

# Keep track of a list of IP addresses that each peer may communicate with to simulate NAT with an Address-Dependent Mapping
adm_ips=()

# Add control and relay server to the list
adm_ips+=($control_ip $relay_ip)

# Two private networks whose NAT mapping and filtering behaviour depends on the system test's parameters
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
    pub_subnet="${pub_prefix}.0/24"
    router_pub_ip="${pub_prefix}.254"

    # Add router's public subnet to list created earlier
    adm_ips+=($pub_subnet)

    if [[ -z $double_nat ]]; then
        # Setup router
        ip netns exec $router_name ./setup_router.sh $router_name $priv_name public $priv_subnet $router_priv_ip $pub_prefix $n_pooling_ips $switch_ip 0

        # Setup private network
        ip netns exec $priv_name ./setup_private.sh $router_name $router_pub_ip $priv_subnet
    else
        # Create namespace for the additional private network
        double_name="double${i}"
        ./create_namespace.sh $double_name

        # Variables related to the additional private network and its router
        double_prefix="172.16.${i}"
        double_subnet="${priv_prefix}.0/24"
        double_ip="${double_prefix}.254"

        # Setup first router
        ip netns exec $router_name ./setup_router.sh $router_name $double_name public $double_subnet $double_ip $pub_prefix $n_pooling_ips $switch_ip 0

        # Setup additional router
        ip netns exec $double_name ./setup_router.sh $double_name $priv_name $router_name $priv_subnet $router_priv_ip $double_prefix $n_pooling_ips $router_pub_ip 1

        # Setup private network
        ip netns exec $priv_name ./setup_private.sh $double_name $double_ip $priv_subnet
    fi

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

echo ${adm_ips[@]}

