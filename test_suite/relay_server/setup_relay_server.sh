#!/usr/bin/env bash

if [[ $# -ne 2 ]]; then
    echo """
Usage: ${0} <RELAY SERVER IP> <RELAY SERVER PORT>"""
    exit 1
fi

relay_ip=$1
relay_port=$2

# Run server once to save output and generate exit code
relay_out=$(sudo timeout --preserve-status -s SIGKILL 1s ./start_relay_server.sh $relay_ip $relay_port 2>& 1)

# Only print public key if server started correctly and was killed by SIGKILL, resulting in exit code 137
if [[ $? -eq 137 ]]; then
    echo $relay_out | grep -Eo "using public key ([0-9a-f]+)" | cut -d ' ' -f4
fi

