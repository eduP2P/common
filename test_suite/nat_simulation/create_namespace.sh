#!/bin/bash

# Usage: ./create_namespace.sh <NAMESPACE NAME>
# This script must be run with root permissions

name=$1

# Deleting the namespace in case it already exists
ip netns list | grep -q $name && ip netns del $name

# Add the namespace
ip netns add $name