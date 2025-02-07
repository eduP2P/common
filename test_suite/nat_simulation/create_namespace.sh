#!/usr/bin/env bash

if [[ $# -ne 1 ]]; then
    echo """
Usage: ${0} <NAMESPACE NAME>

This script must be run with root permissions"""
    exit 1
fi

name=$1

# Delete the namespace in case it already exists
ip netns list | grep -qw $name && ip netns del $name # -w to prevent matching on other namespace which has $name as substring

# Add the namespace
ip netns add $name

# Turn on loopback interface in namespace to allow pinging from inside namespace
ip netns exec $name ip link set lo up

