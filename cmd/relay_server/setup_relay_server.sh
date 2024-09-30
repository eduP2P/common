#!/bin/bash

# Usage: ./setup_relay_server.sh <RELAY SERVER IP> <RELAY SERVER PORT>

relay_ip=$1
relay_port=$2

# Build
go build -o relay_server *.go &> /dev/null

# Run server once to save output and generate exit code
relay_out=$(sudo timeout --preserve-status -s SIGKILL 3s ./start_relay_server.sh $relay_ip $relay_port 2>& 1)

# Only print public key if server started correctly and was killed by SIGKILL, resulting in exit code 137
if [[ $? -eq 137 ]]; then
    echo $relay_out | grep -Eo "using public key ([0-9a-f]+)" | cut -d ' ' -f4
fi

