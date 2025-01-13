#!/bin/bash

usage_str="""
Usage: ${0} [OPTIONAL ARGUMENTS] <NAMESPACE PEER 1> <NAMESPACE PEER 2> <VIRTUAL IP PEER 1> <PERFORMANCE TEST VARIABLE> <PERFORMANCE TEST VALUES> <PERFORMANCE TEST DURATION> <LOG DIRECTORY>

[OPTIONAL ARGUMENTS]:
    -b
        With this flag, eduP2P's performance is compared to the performance of a direct connection, and a connection using only WireGuard
        This flag should only be used when both peers reside in the 'public' network

Executes performance tests between the peers using iperf3, where peer 1 acts as the server and peer 2 as the client
This script must be executed with root permissions

<PERFORMANCE TEST VARIABLE> can be either 'packet_loss' or 'bitrate'

<PERFORMANCE TEST VALUES> should be a comma-separated string of positive real numbers (smaller than 100 in case of packet_loss, since its unit is %)

<PERFORMANCE TEST DURATION> should be a positive integer specifiying the amount of seconds the performance test for each value will take 
"""

# Use functions and constants from util.sh
. ./util.sh

# Validate optional arguments
while getopts ":bh" opt; do
    case $opt in
        b)
            baseline=true
            ;;
        h) 
            echo "$usage_str"
            exit 0
            ;;
        *)
            exit_with_error "invalid option"
            ;;
    esac
done

# Shift positional parameters indexing by accounting for the optional arguments
shift $((OPTIND-1))

# Make sure all required arguments have been passed
if [[ $# -ne 7 ]]; then
    exit_with_error "expected 7 positional parameters, but received $#"
fi

peer1=$1
peer2=$2
peer1_ip=$3
performance_test_var=$4
performance_test_values=$5
performance_test_duration=$6
log_dir=$7

function clean_exit() {
    exit_code=$1

    # Delete baseline WireGuard interfaces and private keys, and kill keep-alive process
    if [[ $baseline == true ]]; then
        for ns in $peer1 $peer2; do
            sudo ip netns exec $ns ip link del wg_$ns
            rm private_$ns
        done

        kill $keep_alive_pid
    fi

    # Undo the restrictive permissions which iperf3 sets on test_dir
    chmod --recursive 777 $test_dir

    exit $exit_code
}

# Measure delay between peers using an HTTP server and curl
function measure_delay() {
    server_ip=$1

    # Start HTTP server on Peer 1 to measure connection delay
    ip netns exec $peer1 python3 -m http.server -b $server_ip 80 &> /dev/null &
    http_pid=$!

    # Variables to store average delay
    avg_delay=0

    # Repeat delay measurement 100 times
    for i in {1..100}; do
        delay_seconds=$(ip netns exec $peer2 curl -s -o /dev/null -w "%{time_total}\n" http://$server_ip)
        delay_milliseconds=$(echo "$delay_seconds * 1000" | bc)
        avg_delay=$(echo "$avg_delay + $delay_milliseconds" | bc)
    done

    # Stop HTTP server
    kill $http_pid

    # Calculate and print average delay
    avg_delay=$(echo "$avg_delay * 0.001" | bc)
    echo "$avg_delay"
}

# Use an inline Python script to store the delay in an iperf3 json file
function store_delay() {
    delay=$1
    file=$2

    python3 -c """
import json

with open('$file', 'r+') as f:
    data = json.load(f)
    data['end']['sum']['delay_ms'] = $delay
    f.seek(0)
    json.dump(data, f, indent=4)"""
}

# Function to do a performance test for one performance test value
function performance_test() {
    test_val=$1
    test_dir=$2
    connection=$3
    server_ip=$4

    # Peer 1 is the iperf3 server
    ip netns exec $peer1 iperf3 -s -B $server_ip -p 12345 -1 --logfile /dev/null & # -1 to close after first connection
    server_pid=$!

    sleep 1s

    # Peer 2 is the iperf3 client
    log_file=$performance_test_var=$test_val.json
    log_path=$test_dir/$connection/$log_file
    mkdir -p $test_dir/$connection

    # Client command depends on performance test variable
    case $performance_test_var in
        "packet_loss")
            ./set_packet_loss.sh 0 # Set packet loss to 0 for quick handshake
            ip netns exec $peer2 iperf3 -c $server_ip -p 12345 -u -t $performance_test_duration --connect-timeout 3000 --json --logfile $log_path -b 100000000 --omit 2 & # Omit first two seconds
            client_pid=$!

            # Make sure packet loss is reset to the correct value before the test results are measured (sleep period is smaller than omit period above)
            sleep 1s
            ./set_packet_loss.sh $test_val
            ;;
        "bitrate")
            bitrate=$(( $test_val * 10**6 )) # Convert to bits/sec
            ip netns exec $peer2 iperf3 -c $server_ip -p 12345 -u -t $performance_test_duration --connect-timeout 3000 --json --logfile $log_path -b $bitrate &
            client_pid=$!
            ;;
    esac

    # Wait until client is done, and throw an error if its execution failed
    wait $client_pid

    if [[ $? -ne 0 ]]; then
        kill $server_pid &> /dev/null
        echo -e "\t$bar ${RED}Performance testing with $performance_test_var = $test_val failed: see $connection/$log_file for the iperf3 error message${NC}"
        clean_exit 1
    fi

    # Wait until server has closed
    wait $server_pid

    # Measure delay and store it in the iperf3 log file
    delay=$(measure_delay $server_ip)
    store_delay $delay $log_path
}

# Set up WireGuard connection between the peers (for performance test baseline)
function wg_setup() {
    # Counter for virtual IP addresses
    i=1

    # Generate private key and interface for each peer
    for ns in $peer1 $peer2; do
        wg_iface=wg_$ns
        priv_key=private_$ns
        
        wg genkey | tee $priv_key &> /dev/null # Combination of tee and redirect to /dev/null is necessary to avoid weird behaviour caused by redirecting in a script run with sudo
        ip netns exec $ns ip link add $wg_iface type wireguard
        ip netns exec $ns ip addr add 10.0.0.$i/24 dev $wg_iface
        ip netns exec $ns wg set $wg_iface private-key ./$priv_key
        ip netns exec $ns ip link set $wg_iface up

        let "i++"
    done

    # Store peers' public keys
    pub1=$(wg pubkey < private_$peer1)
    pub2=$(wg pubkey < private_$peer2)

    # Store peers's listening ports
    port1=$(sudo ip netns exec $peer1 wg show wg_$peer1 listen-port)
    port2=$(sudo ip netns exec $peer2 wg show wg_$peer2 listen-port)

    # Add the peers to each others' WireGuard configurations
    ip netns exec $peer1 wg set wg_$peer1 peer $pub2 allowed-ips 10.0.0.2/32 endpoint 192.168.2.254:$port2
    ip netns exec $peer2 wg set wg_$peer2 peer $pub1 allowed-ips 10.0.0.1/32 endpoint 192.168.1.254:$port1
}

# Directory to store performance test results
performance_test_dir=$log_dir/performance_tests_$performance_test_var

# Replace commas by spaces to convert string to array
performance_test_values=$(echo $performance_test_values | tr ',' ' ') 
performance_test_values=($performance_test_values)

# Variables to display a progress bar
n_values=${#performance_test_values[@]}
progress=0

# For the baseline comparison, we need the peers' public IPs, which are also needed to setup a WireGuard connection between them
if [[ $baseline == true ]]; then
    peer1_pub_ip=$(ip netns exec $peer1 ip address | grep -Eo "inet 192.168.[0-9.]+" | cut -d ' ' -f2)
    peer2_pub_ip=$(ip netns exec $peer2 ip address | grep -Eo "inet 192.168.[0-9.]+" | cut -d ' ' -f2)

    if [[ -z $peer1_pub_ip || -z $peer2_pub_ip ]]; then
        exit_with_error "For at least one of the two peers, a public IP could not be found"
    fi

    wg_setup

    # Keep eduP2P connection alive during baseline tests by continuously sending pings
    ip netns exec $peer2 ping $peer1_ip &> /dev/null &
    keep_alive_pid=$!
fi

# Iterate over performance test values
for performance_test_val in ${performance_test_values[@]}; do
    bar=$(progress_bar $progress $n_values)
    echo -ne "\033[2K\t$bar Performance testing with $performance_test_var = $performance_test_val\r" # \033[2K = Ctrl+K, clears rest of line from cursor; \r returns to beginning of line

    # Run performance test for eduP2P
    performance_test $performance_test_val $performance_test_dir "eduP2P" $peer1_ip

    # If -b is set, the performance test is repeated over a direct/WireGuard connection instead of over the eduP2P connection
    if [[ $baseline == true ]]; then
        performance_test $performance_test_val $performance_test_dir "Direct" $peer1_pub_ip
        performance_test $performance_test_val $performance_test_dir "WireGuard" 10.0.0.1
    fi

    let "progress++"
done

bar=$(progress_bar $n_values $n_values)
echo -e "\t$bar Performance testing with $performance_test_var finished"
clean_exit 0

