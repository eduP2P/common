#!/bin/bash

if [[ $# -ne 2 ]]; then
    echo """
Usage: ${0} <RELAY SERVER IP> <RELAY SERVER PORT>"""
    exit 1
fi

sudo ip netns exec relay ./relay_server -c ./relay.json -a "${1}:${2}" -stun-port 3478 