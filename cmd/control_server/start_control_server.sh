#!/bin/bash

# Usage: ./start_control_server.sh <CONTROL SERVER PORT>

./control_server -c ./control.json -a ":${1}"

