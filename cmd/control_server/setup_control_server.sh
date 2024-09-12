#!/bin/bash

# Usage: ./setup_control_server.sh <CONTROL SERVER PORT>
control_port=$1

# Build
go build -o control_server *.go

# Run once to save and store public key
timeout 3s ./start_control_server.sh $control_port &> temp # Redirect STDERR to temporary file
control_pub_key=$(grep -o "using public key [0-9a-f]\+" temp | cut -d ' ' -f4) # Extract public key from temp
rm temp

# Print public key
echo $control_pub_key
