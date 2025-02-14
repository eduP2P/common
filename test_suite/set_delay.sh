#!/usr/bin/env bash

usage_str="""
Usage: ${0} <DELAY>

<DELAY> should be an even integer specified in milliseconds"""

delay=$1

# Make sure delay is an integer
int_regex="^[0-9]+$"

if [[ $# -ne 1 || ! ( $delay =~ $int_regex) ]]; then
    echo $usage_str
    exit 1
fi

# Iterate over all possible hosts in the simulated network setup
for host in control relay router1_pub router2_pub private1_peer1 private1_peer2 private2_peer1 private2_peer2; do
    # Derive namespace name from host
    router_host_regex="(router[0-9]+)_pub"

    if [[ $host =~ $router_host_regex ]]; then
        ns=${BASH_REMATCH[1]}
    else
        ns=$host
    fi

    sudo ip netns exec $ns tc qdisc replace dev $host root netem delay ${delay}ms
done
