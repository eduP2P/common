#!/bin/bash

# Usage: ./setup_control_server.sh <CONTROL SERVER IP> <CONTROL SERVER PORT>

control_ip=$1
control_port=$2

# Build
go build -o control_server *.go

# Run once to save public key and generate exit code
control_out=$(sudo timeout --preserve-status -s SIGKILL 3s ./start_control_server.sh $control_ip $control_port 2>& 1)

# Only print public key if server started correctly and was killed by SIGKILL, resulting in exit code 137
if [[ $? -eq 137 ]]; then
    echo $control_out | grep -Eo "using public key ([0-9a-f]+)" | cut -d ' ' -f4
fi

