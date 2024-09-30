#!/bin/bash

# Usage: ./start_control_server.sh CONTROL SERVER IP> <CONTROL SERVER PORT>

sudo ip netns exec public ./control_server -c ./control.json -a "${1}:${2}"

