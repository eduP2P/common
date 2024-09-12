#!/bin/bash

# Usage: ./start_relay_server.sh <RELAY SERVER IP> <RELAY SERVER PORT>

./relay_server -c ./relay.json -a "${1}:${2}" -stun-port 3478 