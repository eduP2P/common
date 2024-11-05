#!/bin/bash

usage_str="""
Usage: ${0} [OPTIONAL ARGUMENTS] <PEER ID> <TEST TARGET> <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <LOG LEVEL> <PERFORMANCE TEST ROLE> [WIREGUARD INTERFACE]

[OPTIONAL ARGUMENTS] can be provided for a performance test:
    -k <packet_loss|bitrate>
    -v <comma-separated string of positive real numbers>
    -d <seconds>

<LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)

<PERFORMANCE TEST ROLE> should be either 'client', 'server' or 'none'

If [WIREGUARD INTERFACE] is not provided, this peer will use userspace WireGuard"""

# Use functions and constants from util.sh
. ../../test_suite/util.sh

# Validate optional arguments
while getopts ":k:v:d:h" opt; do
    case $opt in
        k)
            performance_test_var=$OPTARG
            validate_str $performance_test_var "^packet_loss|bitrate$"
            ;;
        v)
            performance_test_values=$OPTARG
            real_regex="[0-9]+(.[0-9]+)?"
            validate_str "$performance_test_values" "^$real_regex(,$real_regex)*$"
            ;;
        d)
            performance_test_duration=$OPTARG
            validate_str $performance_test_duration "^[0-9]+*$"
            ;;
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

# Make sure all required positional parameters have been passed
min_req=8
max_req=9

if [[ $# < $min_req || $# > max_req ]]; then
    print_err "expected $min_req or $max_req positional parameters, but received $#"
    exit 1
fi

id=$1
test_target=$2
control_pub_key=$3
control_ip=$4
control_port=$5
log_lvl=$6
log_dir=$7
performance_test_role=$8
wg_interface=$9

# Create WireGuard interface if wg_interface is set
if [[ -n $wg_interface ]]; then
    sudo ip link add $wg_interface type wireguard
    sudo wg set $wg_interface listen-port 0 # 0 means port is chosen randomly
    sudo ip link set $wg_interface up
fi

# Create temporary file to store test_client CLI output
out="test_client_out_${id}.txt"

# Store test_client output in a temporary file
(sudo ./test_client --control-host=$control_ip --control-port=$control_port --control-key=control:$control_pub_key --ext-wg-device=$wg_interface --log-level=$log_lvl --config=$id.json 2>&1 | tee $out &)

function clean_exit () {
    exit_code = $1

    # Remove temporary test_client output file
    sudo rm $out

    # Terminate test_client
    test_client_pid=$(pgrep test_client) && sudo kill $test_client_pid

    # Remove http server output files
    if [[ -n $http_ipv4_out ]]; then rm $http_ipv4_out; fi
    if [[ -n $http_ipv6_out ]]; then rm $http_ipv6_out; fi

    # Kill http servers 
    if [[ -n $http_ipv4_pid ]]; then kill $http_ipv4_pid; fi
    if [[ -n $http_ipv6_pid ]]; then kill $http_ipv6_pid; fi

    # Delete external WireGuard interface in case external WireGuard was used
    if [[ -n $wg_interface ]]; then sudo ip link del $wg_interface; fi

    exit $exit_code
}

# Get own virtual IPs and peer's virtual IPs; method is different for exernal WireGuard vs userspace WireGuard
if [[ -n $wg_interface ]]; then
    # Store virtual IPs as "<IPv4> <IPv6>"" when they are logged
    ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    if [[ -z $ips ]]; then echo "TS_FAIL: could not find own virtual IPs in logs"; clean_exit 1; fi

    # Split IPv4 and IPv6
    ipv4=$(echo $ips | cut -d ' ' -f1) 
    ipv6=$(echo $ips | cut -d ' ' -f2)

    # Add virtual IPs to WireGuard interface
    sudo ip address add $ipv4 dev $wg_interface
    sudo ip address add $ipv6 dev $wg_interface
    
    # Remove network prefix length from own virtual IPs
    ipv4=$(echo $ipv4 | cut -d '/' -f1)
    ipv6=$(echo $ipv6 | cut -d '/' -f1)

    # Wait until timeout or until WireGuard interface is updated to contain peer's virtual IPs
    timeout=10
    peer_ips=$(wg show $wg_interface allowed-ips | cut -d$'\t' -f2) # IPs are shown as "<wg pubkey>\t<IPv4> <IPv6>"

    while [[ -z $peer_ips ]]; do
        sleep 1s
        let "timeout--"

        if [[ $timeout -eq 0 ]]; then
            echo "TS_FAIL: timeout waiting for eduP2P to update the WireGuard interface"
            clean_exit 1
        fi

        peer_ips=$(wg show $wg_interface allowed-ips | cut -d$'\t' -f2)
    done

    # Split IPv4 and IPv6, and remove network prefix length
    peer_ipv4=$(echo $peer_ips | cut -d ' ' -f1 | cut -d '/' -f1) 
    peer_ipv6=$(echo $peer_ips | cut -d ' ' -f2 | cut -d '/' -f1)
else
    # Wait until timeout or until TUN interface created with userspace WireGuard is updated to contain peer's virtual IPs
    timeout=10
    
    while ! ip address show ts0 | grep -Eq "inet [0-9.]+"; do
        sleep 1s
        let "timeout--"

        if [[ $timeout -eq 0 ]]; then
            echo "TS_FAIL: timeout waiting for eduP2P to update the WireGuard interface"
            clean_exit 1
        fi
    done

    # Extract own virtual IPs from TUN interface
    ipv4=$(ip address show ts0 | grep -Eo "inet [0-9.]+" | cut -d ' ' -f2)
    ipv6=$(ip address show ts0 | grep -Eo -m 1 "inet6 [0-9a-f:]+" | cut -d ' ' -f2)

    # Store peer IPs as "<IPv4> <IPv6>"" when they are logged
    peer_ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*IPv4:(\S+) IPv6:(\S+).*/{s//\1 \2/p;q}")

    if [[ -z $peer_ips ]]; then echo "TS_FAIL: could not find peer's virtual IPs in logs"; clean_exit 1; fi

    # Split IPv4 and IPv6
    peer_ipv4=$(echo $peer_ips | cut -d ' ' -f1)
    peer_ipv6=$(echo $peer_ips | cut -d ' ' -f2)
fi

# Start HTTP servers on own virtual IPs for peer to access, and save their pids to kill them during cleanup
http_ipv4_out="http_ipv4_output_${id}.txt"
python3 -m http.server -b $ipv4 80 &> $http_ipv4_out &
http_ipv4_pid=$!

http_ipv6_out="http_ipv6_output_${id}.txt"
python3 -m http.server -b $ipv6 80 &> $http_ipv6_out &
http_ipv6_pid=$!

# Try connecting to peer's HTTP server hosted on IP addres
function try_connect() {
    peer_addr=$1

    if ! curl --retry 3 --retry-all-errors -I -s -o /dev/null $peer_addr; then
        echo "TS_FAIL: could not connect to peer's HTTP server with address: ${peer_ip}"
        clean_exit 1
    fi
}

try_connect "http://${peer_ipv4}"

# Peers try to establish a direct connection after initial connection; if expecting a direct connection, give them some time to establish one
if [[ $test_target == "TS_PASS_DIRECT" ]]; then
    timeout 10s tail -f -n +1 $out | sed -n "/ESTABLISHED direct peer connection/q"
fi

try_connect "http://[${peer_ipv6}]"

# Wait until timeout or until peer connected to server (peer's IP will appear in server output)
timeout 10s tail -f -n +1 $http_ipv4_out | sed -n "/${peer_ipv4}/q"
timeout 10s tail -f -n +1 $http_ipv6_out | sed -n "/${peer_ipv6}/q"

# Optional performance test with iperf
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

    # Run performance test
    connect_timeout=3

    case $performance_test_role in
    "client") 
        logfile=$performance_test_dir/$performance_test_var=$performance_test_val.json

        # Retry until server is listening or until timeout
        while ! iperf3 -c $peer_ipv4 -p 12345 -u -t $performance_test_duration -b $bitrate --json --logfile $logfile; do
            sleep 1s
            let "connect_timeout--"

            if [[ $connect_timeout -eq 0 ]]; then
                echo "TS_FAIL: timeout while trying to connect to peer's iperf server to test performance"
                clean_exit 1
            fi

            rm $logfile # File is created and contains an error message, delete for next iteration
        done 
        ;;
    "server") 
        test_timeout=$(($connect_timeout + $performance_test_duration + 1))
        timeout ${test_timeout}s iperf3 -s -B $ipv4 -p 12345 -1 &> /dev/null # -1 to close after first connection

        if [[ $? -ne 0 ]]; then
            echo "TS_FAIL: timeout while listening on iperf server to test performance"
            clean_exit 1
        fi
        ;;
esac
}

performance_test_dir=$log_dir/performance_tests_$performance_test_var
performance_test_values=$(echo $performance_test_values | tr ',' ' ') # Replace commas by spaces to iterate over each value easily

for performance_test_val in $performance_test_values; do
    mkdir -p $performance_test_dir
    performance_test $performance_test_val $performance_test_dir
done

echo "TS_PASS"
clean_exit 0