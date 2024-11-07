#!/bin/bash

# Usage: execute ". /path/to/util.sh" in another script to be able to use this script's function in that other script

# Constants for colored text in output
RED="\033[0;31m"
GREEN="\033[0;32m"
NC="\033[0m" # No color

function print_err() {
    err_reason=$1

    echo -e "${RED}Error: $err_reason; run $0 with the -h flag to receive usage information${NC}"
}

function validate_str() {
    str=$1
    regex=$2

    if [[ ! $str =~ $regex ]]; then
        print_err "the argument \"$str\" is invalid"
        exit 1
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