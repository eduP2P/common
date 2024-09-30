#!/bin/bash

# Usage: ./setup_toverstok.sh <PEER ID> <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <LOG LEVEL> <WIREGUARD INTERFACE> 
# <LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages)
# <WIREGUARD INTERFACE> is optional, if it is not set eduP2P is configured with userspace WireGuard

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

    # Delete external WireGuard interface in case external WireGuard was used
    if [[ -n $wg_interface ]]; then sudo ip link del $wg_interface; fi
}

# Remove pipe and kill background processes when script finishes
trap cleanup EXIT

# Generate commands from template and put them in the pipe
while read line; do
    eval $line > $pipe
done < commands_template.txt


# If wg_interface is set, eduP2P will print some commands to configure the WireGuard interface
if [[ -n $wg_interface ]]; then
    # Store IPs as "<IPv4> <IPv6>"" when they are logged
    ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    if [[ -z $ips ]]; then echo "TS_FAIL: could not find own virtual IPs in logs"; exit 1; fi

    ipv4=$(echo $ips | cut -d ' ' -f1) 
    ipv6=$(echo $ips | cut -d ' ' -f2)

    # Add IPs to WireGuard interface
    sudo ip address add $ipv4 dev $wg_interface
    sudo ip address add $ipv6 dev $wg_interface

    # Sleep for short duration to give toverstok time to update WireGuard interface
    sleep 5s

    # Extract peer's virtual IP address from WireGuard interface
    peer_ipv4=$(wg | grep -Eo "allowed ips: [0-9.]+" | cut -d ' ' -f3)
    peer_ipv6=$(wg | grep -Eo "allowed ips: (\S+) [0-9a-f:]+" | cut -d ' ' -f4)
# When using userspace WireGuard, we can skip the configuration but need to extract the peer's virtual IPs from the logs
else
    # Store peer IPs as "<IPv4> <IPv6>"" when they are logged
    peer_ips=$(timeout 10s tail -n +1 -f $out | sed -rn "/.*IPv4:(\S+) IPv6:(\S+).*/{s//\1 \2/p;q}")

    if [[ -z $peer_ips ]]; then echo "TS_FAIL: could not find peer's virtual IPs in logs"; exit 1; fi

    peer_ipv4=$(echo $peer_ips | cut -d ' ' -f1)
    peer_ipv6=$(echo $peer_ips | cut -d ' ' -f2)
fi

# Try to ping the peer's IPv4 address
if ! ping -c 5 $peer_ipv4 &> /dev/null; then
    echo "TS_FAIL: IPv4 ping failed with IP address: ${peer_ipv4}"
    exit 1
fi 

# Try to ping the peer's IPv6 address
if ! ping -c 5 $peer_ipv6 &> /dev/null; then
    echo "TS_FAIL: IPv6 ping failed with IP address: ${peer_ipv6}"
    exit 1
fi   

echo "TS_PASS"

# Sleep for short duration to give other peer time to ping
sleep 10s