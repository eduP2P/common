#!/bin/bash

usage_str="""
Usage: ${0} <NAMESPACE PEER 1> <NAMESPACE PEER 2> <IP PEER 1> <PERFORMANCE TEST VARIABLE> <PERFORMANCE TEST VALUES> <PERFORMANCE TEST DURATION> <LOG DIRECTORY>

Executes performance tests between the peers using iperf3, where peer 1 acts as the server and peer 2 as the client
This script must be executed with root permissions

<PERFORMANCE TEST VARIABLE> can be either 'packet_loss' or 'bitrate'

<PERFORMANCE TEST VALUES> should be a comma-separated string of positive real numbers (smaller than 100 in case of packet_loss, since its unit is %)

<PERFORMANCE TEST DURATION> should be a positive integer specifiying the amount of seconds the performance test for each value will take 
"""

# Use functions and constants from util.sh
. ./util.sh

# Validate optional arguments
while getopts ":h" opt; do
    case $opt in
        h) 
            echo "$usage_str"
            exit 0
            ;;
        *)
            print_err "invalid option -$opt"
            exit 1
            ;;
    esac
done

# Shift positional parameters indexing by accounting for the optional arguments
shift $((OPTIND-1))

# Make sure all required arguments have been passed
if [[ $# -ne 7 ]]; then
    print_err "expected 7 positional parameters, but received $#"
    exit 1
fi

peer1=$1
peer2=$2
peer1_ip=$3
performance_test_var=$4
performance_test_values=$5
performance_test_duration=$6
log_dir=$7

# Function to do a performance test for one performance test value
function performance_test () {
    performance_test_val=$1
    performance_test_dir=$2

    # Default values
    bitrate=$(( 10**6 )) # Default iperf UDP bitrate is 1 Mbps

    # Assign performance_test_val to performance_test_var
    case $performance_test_var in
        "packet_loss")
            ./set_packet_loss.sh $performance_test_val
            ;;
        "bitrate")
            bitrate=$(( $performance_test_val * 10**6 )) # Convert to bits/sec
            ;;
    esac

    # Peer 1 is the iperf3 server
    ip netns exec $peer1 iperf3 -s -B $peer1_ip -p 12345 -1 --logfile /dev/null & # -1 to close after first connection
    server_pid=$!

    # Give server time to start
    sleep 1s

    # Peer 2 is the iperf3 client
    log_file=$performance_test_var=$performance_test_val.json
    log_path=$performance_test_dir/$log_file
    ip netns exec $peer2 iperf3 -c $peer1_ip -p 12345 -u -t $performance_test_duration -b $bitrate --connect-timeout 1000 --json --logfile $log_path

    if [[ $? -ne 0 ]]; then
        kill $server_pid &> /dev/null
        echo -e "\t$bar ${RED}Performance testing with $performance_test_var = $performance_test_val failed: see $log_file for the iperf3 error message${NC}"
        exit 1
    fi
    
    # Wait until server has closed
    wait $server_pid
}

# Directory to store performance test results
performance_test_dir=$log_dir/performance_tests_$performance_test_var
mkdir $performance_test_dir

# Replace commas by spaces to conver
performance_test_values=$(echo $performance_test_values | tr ',' ' ') 

# Convert string to array
performance_test_values=($performance_test_values)
n_values=${#performance_test_values[@]}
progress=0

# Iterate over performance test values
for performance_test_val in ${performance_test_values[@]}; do
    bar=$(progress_bar $progress $n_values)
    echo -ne "\t$bar Performance testing with $performance_test_var = $performance_test_val\r"
    performance_test $performance_test_val $performance_test_dir
    let "progress++"
done

bar=$(progress_bar $n_values $n_values)
echo -e "\t$bar Performance testing with $performance_test_var finished"