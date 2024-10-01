#!/bin/bash

if [[ $# -ne 1 ]]; then
    echo """
Usage: ${0} <NAMESPACE NAME>

This script must be run with root permissions"""
    exit 1
fi

name=$1

# Deleting the namespace in case it already exists
ip netns list | grep -q $name && ip netns del $name

# Add the namespace
ip netns add $name