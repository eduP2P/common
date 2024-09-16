#!/bin/bash

# Usage: ./setup_control_server.sh <CONTROL SERVER PORT>
control_port=$1

# Build
go build -o control_server *.go

# Run once to save public key and generate exit code
control_out=$(timeout --preserve-status -s SIGINT 3s ./start_control_server.sh $control_port 2>& 1)

# Only print public key if server started correctly
if [[ $? -eq 0 ]]; then
    echo $control_out | grep -Eo "using public key ([0-9a-f]+)" | cut -d ' ' -f4
fi

