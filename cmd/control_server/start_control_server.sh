#!/bin/bash

if [[ $# -ne 2 ]]; then
    echo """
Usage: ${0} <CONTROL SERVER IP> <CONTROL SERVER PORT>"""
    exit 1
fi

sudo ip netns exec public ./control_server -c ./control.json -a "${1}:${2}"

