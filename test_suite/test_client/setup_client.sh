#!/usr/bin/env bash

usage_str="""
Usage: ${0} <PEER ID> <PEER NAMESPACE> <TEST TARGET> <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <LOG LEVEL> [WIREGUARD INTERFACE]

<LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)

If [WIREGUARD INTERFACE] is not provided, this peer will use userspace WireGuard"""

# Use functions and constants from util.sh
. ../../test_suite/util.sh

# Validate optional arguments
while getopts ":h" opt; do
    case $opt in
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

# Make sure all required positional parameters have been passed
min_req=7
max_req=8

if [[ $# < $min_req || $# > $max_req ]]; then
    exit_with_error "expected $min_req or $max_req positional parameters, but received $#"
fi

id=$1
peer_ns=$2
test_target=$3
control_pub_key=$4
control_ip=$5
control_port=$6
log_lvl=$7
wg_interface=$8

# Create WireGuard interface if wg_interface is set
if [[ -n $wg_interface ]]; then
    sudo ip link add $wg_interface type wireguard
    sudo wg set $wg_interface listen-port 0 # 0 means port is chosen randomly
    sudo ip link set $wg_interface up
fi

# Create temporary file to store test_client output
out="test_client_out_${id}.txt"
touch $out

# Run test_client and store its output in the temporary file
(sudo ./test_client --control-host=$control_ip --control-port=$control_port --control-key=control:$control_pub_key --ext-wg-device=$wg_interface --log-level=$log_lvl --config=$id.json 2>&1 | tee $out &)

function clean_exit() {
    exit_code=$1

    # Remove temporary test_client output file
    sudo rm $out

    # Remove http server output file if it exists
    rm $http_ipv6_out &> /dev/null

    # Kill http servers if they are running
    kill $http_ipv4_pid &> /dev/null
    kill $http_ipv6_pid &> /dev/null

    exit $exit_code
}

# Also call clean_exit when killed from parent script
trap "clean_exit 1" SIGTERM

# Get own virtual IPs and peer's virtual IPs with external WireGuard
if [[ -n $wg_interface ]]; then
    # Store virtual IPs as "<IPv4> <IPv6>" when they are logged
    ips=$(timeout 10s tail -n +1 -f -s 0.1 $out | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    if [[ -z $ips ]]; then 
        echo "TS_FAIL: could not find own virtual IPs in logs"
        clean_exit 1
    fi

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
        sleep 0.1s
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
# Get own virtual IPs and peer's virtual IPs with userspace WireGuard
else
    # Wait until timeout, or until automatically created TUN interface is updated to contain peer's virtual IPs
    timeout=10
    
    while ! ip address show ts0 | grep -Eq "inet [0-9.]+"; do
        sleep 0.1s
        let "timeout--"

        if [[ $timeout -eq 0 ]]; then
            echo "TS_FAIL: timeout waiting for eduP2P to update the WireGuard interface"
            clean_exit 1
        fi
    done

    # Extract own virtual IPs from TUN interface
    ipv4=$(extract_ipv4 $peer_ns ts0)
    ipv6=$(extract_ipv6 $peer_ns ts0)

    # Store peer IPs as "<IPv4> <IPv6>" when they are logged
    peer_ips=$(timeout 10s tail -f -n +1 -s 0.1 $out | sed -rn "/.*IPv4:(\S+) IPv6:(\S+).*/{s//\1 \2/p;q}")

    if [[ -z $peer_ips ]]; then 
        echo "TS_FAIL: could not find peer's virtual IPs in logs"
        clean_exit 1
    fi

    # Split IPv4 and IPv6
    peer_ipv4=$(echo $peer_ips | cut -d ' ' -f1)
    peer_ipv6=$(echo $peer_ips | cut -d ' ' -f2)
fi

# Necessary to avoid failures with hairpinning tests, probably caused by delay in adding nftables rules to simulate hairpinning
sleep 0.5s

# Start HTTP servers on own virtual IPs for peer to access, and save their pids to kill them during cleanup
http_ipv4_out="http_ipv4_output_${id}.txt"
python3 -m http.server -b $ipv4 80 &> /dev/null &
http_ipv4_pid=$!

http_ipv6_out="http_ipv6_output_${id}.txt"
python3 -m http.server -b $ipv6 80 &> $http_ipv6_out &
http_ipv6_pid=$!

# Desynchronize peers to avoid error and subsequent recovery delay caused by handshake initation in both directions at same time
if [[ $id == "peer1" ]]; then
    sleep 0.1s
fi

# Try connecting to peer's HTTP server hosted on IP addres
function try_connect() {
    peer_addr=$1

    if ! curl --retry 3 --retry-all-errors -I -s -o /dev/null $peer_addr; then
        echo "TS_FAIL: could not connect to peer's HTTP server with address: ${peer_addr}"
        clean_exit 1
    fi
}

try_connect "http://${peer_ipv4}"

# Peers try to establish a direct connection after initial connection; if expecting a direct connection, give them some time to establish one
if [[ $test_target == "TS_PASS_DIRECT" ]]; then
    timeout 10s tail -f -n +1 -s 0.1 $out | sed -n "/ESTABLISHED direct peer connection/q"
fi

try_connect "http://[${peer_ipv6}]"

# Wait until timeout or until peer connected to second server (peer's IP will appear in server output)
timeout 10s tail -f -n +1 -s 0.1 $http_ipv6_out | sed -n "/${peer_ipv6}/q"

echo "TS_PASS"
clean_exit 0