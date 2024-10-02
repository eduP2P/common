#!/bin/bash

if [[ $# < 5 || $# > 6 ]]; then
    echo """
Usage: ${0} <PEER ID> <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <LOG LEVEL> [WIREGUARD INTERFACE]

<LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)
If [WIREGUARD INTERFACE] is not provided, eduP2P is configured with userspace WireGuard"""
    exit 1
fi

id=$1
control_pub_key=$2
control_ip=$3
control_port=$4
log_lvl=$5
wg_interface=$6

# Create WireGuard interface if wg_interface is set
if [[ -n $wg_interface ]]; then
    sudo ip link add $wg_interface type wireguard
    sudo wg set $wg_interface listen-port 0 # 0 means port is chosen randomly
    sudo ip link set $wg_interface up
fi

# Create pipe to redirect input to toverstok CLI
pipe="toverstok_in_${id}"
mkfifo $pipe

# Create temporary file to store toverstok CLI output
out="toverstok_out_${id}.txt"

# Redirect pipe to toverstok binary, and also store its output in a temporary file
(sudo ./toverstok < $pipe 2>&1 | tee $out &) # Use sed to copy the combined output stream to the specified log file, until the test suite's exit code is found &)

# Ensure pipe remains open by continuously feeding input in background
(
    while true; do
        echo "" > $pipe
    done
)&

# Save pid of above background process to kill later
feed_pipe_pid=$!

function cleanup () {
    # Kill process continuosly feeding input to toverstok
    sudo kill $feed_pipe_pid

    # Remove pipe
    sudo rm $pipe

    # Remove temporary toverstok output file
    sudo rm $out

    # Terminate toverstok, which remains open with userspace WireGuard
    toverstok_pid=$(pgrep toverstok) && sudo kill $toverstok_pid

    # Remove http server output files
    rm $http_ipv4_out
    rm $http_ipv6_out

    # Kill http servers 
    if [[ -n $http_ipv4_pid ]]; then kill $http_ipv4_pid; fi
    if [[ -n $http_ipv6_pid ]]; then kill $http_ipv6_pid; fi

    # Delete external WireGuard interface in case external WireGuard was used
    if [[ -n $wg_interface ]]; then sudo ip link del $wg_interface; fi
}

# Remove pipe and kill background processes when script finishes
trap cleanup EXIT

# Generate commands from template and put them in the pipe
while read line; do
    eval $line > $pipe
done < commands_template.txt

# Get own virtual IPs and peer's virtual IPs; method is different for exernal WireGuard vs userspace WireGuard
if [[ -n $wg_interface ]]; then
    # Store virtual IPs as "<IPv4> <IPv6>"" when they are logged
    ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    if [[ -z $ips ]]; then echo "TS_FAIL: could not find own virtual IPs in logs"; exit 1; fi

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
        timeout=$(($timeout-1))

        if [[ $timeout -eq 0 ]]; then
            echo "TS_FAIL: timeout waiting for eduP2P to update the WireGuard interface"
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
        timeout=$(($timeout-1))

        if [[ $timeout -eq 0 ]]; then
            echo "TS_FAIL: timeout waiting for eduP2P to update the WireGuard interface"
        fi
    done

    # Extract own virtual IPs from TUN interface
    ipv4=$(ip address show ts0 | grep -Eo "inet [0-9.]+" | cut -d ' ' -f2)
    ipv6=$(ip address show ts0 | grep -Eo -m 1 "inet6 [0-9a-f:]+" | cut -d ' ' -f2)

    # Store peer IPs as "<IPv4> <IPv6>"" when they are logged
    peer_ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*IPv4:(\S+) IPv6:(\S+).*/{s//\1 \2/p;q}")

    if [[ -z $peer_ips ]]; then echo "TS_FAIL: could not find peer's virtual IPs in logs"; exit 1; fi

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

# Try connecting to peer's HTTP server hosted on its IPv4 address
if ! curl --retry 3 --retry-all-errors -I -s -o /dev/null "http://${peer_ipv4}"; then
    echo "TS_FAIL: could not connect to peer's HTTP server on IP address: ${peer_ipv4}"
fi

# Wait to give peer time to establish direct connection
timeout 10s tail -f -n +1 $out | sed -n "/ESTABLISHED direct peer connection/q"

# Try connecting to peer's HTTP server hosted on its IPv4 address
if ! curl --retry 3 --retry-all-errors -I -s -o /dev/null -6 "http://[${peer_ipv6}]"; then
    echo "TS_FAIL: could not connect to peer's HTTP server on IP address: ${peer_ipv6}"
fi

# Wait until timeout or until peer connected to server (peer's IP will appear in server output)
timeout 10s tail -f -n +1 $http_ipv4_out | sed -n "/${peer_ipv4}/q"
timeout 10s tail -f -n +1 $http_ipv6_out | sed -n "/${peer_ipv6}/q"

echo "TS_PASS"