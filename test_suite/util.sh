#!/usr/bin/env bash

# Usage: execute ". /path/to/util.sh" in another script to be able to use this script's functions and variables in that other script

# Constants for colored text in output
RED="\033[0;31m"
GREEN="\033[0;32m"
NC="\033[0m" # No color

function exit_with_error() {
    err_reason=$1

    echo -e "${RED}Error: $err_reason; run $0 with the -h flag to receive usage information${NC}"
    exit 1
}

function validate_str() {
    str=$1
    regex=$2

    if [[ ! $str =~ $regex ]]; then
        exit_with_error "the argument \"$str\" is invalid"
    fi
}

function repeat(){
	n=$1
	str=$2
	range=$(seq 1 $n)

	for i in $range; do 
        echo -n $str
    done
}

function progress_bar() {
    progress=$1
    total=$2

    done=$(repeat $progress '▇')
    todo=$(repeat $(($total - $progress)) '-')

    echo "|${done}${todo}|"
}

function extract_ipv4() {
    namespace=$1
    interface=$2

    ipv4=$(sudo ip netns exec $namespace ip address show $interface | grep -Eo "inet [0-9.]+" | cut -d ' ' -f2)

    echo $ipv4
}

function extract_ipv6() {
    namespace=$1
    interface=$2

    ipv6=$(sudo ip netns exec $namespace ip address show $interface | grep -Eo -m 1 "inet6 [0-9a-f:]+" | cut -d ' ' -f2) # -m 1 to avoid matching IPv6 Link-Local address
    
    echo $ipv6
}