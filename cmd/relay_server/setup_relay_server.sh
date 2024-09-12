#!/bin/bash

# Usage: ./setup_relay_server.sh <RELAY SERVER IP> <RELAY SERVER PORT>
relay_ip=$1
relay_port=$2

# Build
go build -o relay_server *.go

# Run once to save and store public key
timeout 3s ./start_relay_server.sh $relay_ip $relay_port &> temp # Redirect STDERR to temporary file
relay_pub_key=$(grep -o "using public key [0-9a-f]\+" temp | cut -d ' ' -f4) # Extract public key from temp
rm temp

# Print public key
echo $relay_pub_key
