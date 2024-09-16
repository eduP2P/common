#!/bin/bash

# Usage: ./setup_toverstok.sh <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <WIREGUARD INTERFACE> 
# <WIREGUARD INTERFACE> is optional, if it is not set eduP2P is configured with userspace WireGuard

control_pub_key=$1
control_ip=$2
control_port=$3
wg_interface=$4

# Create WireGuard interface if wg_interface is set
if [[ -n $wg_interface ]]
then
    ip link add $wg_interface type wireguard
fi

# Create pipe to redirect input to toverstok CLI
mkfifo toverstok_in

# Redirect toverstok_in to toverstok binary, and also write this binary's STDOUT and STDERR to toverstok_log.txt
(./toverstok < toverstok_in 2>&1 | tee toverstok_log.txt &)

# Ensure pipe remains open by continuously feeding input in background
(
    while true; do
        echo "" > toverstok_in
    done
)&

# Remove pipe and kill background process when script finishes
trap "kill %1; rm toverstok_in" EXIT

# Create log file for toverstok
touch toverstok_log.txt

# Generate commands from template and put them in the pipe
while read line; do
    eval $line > toverstok_in
done < commands_template.txt

# Function to reconfigure eduP2P's WireGuard interface; I did not get it to work without these changes
function fix_wg_interface() {
    # Store peer IP address when it is logged as "Endpoints:[<IP>"
    peer_ip=$(timeout 10s tail -n +1 -f toverstok_log.txt | sed -rn "/.*Endpoints:\[([0-9.]+).*/{s//\1/p;q}")
    if [[ -z $peer_ip ]]; then echo "TS_FAIL: could not find peer endpoint in logs"; exit 1; fi 

    # Fix eduP2P's WireGuard interface
    port=65535
    wg set $wg_interface listen-port $port # Port is hardcoded for now

    peer_pub_key=$(wg | grep -o "peer: \S*" | cut -d ' ' -f2)
    wg set $wg_interface peer $peer_pub_key endpoint "${peer_ip}:${port}"

    ip link set $wg_interface up
}

# If wg_interface is set, eduP2P will print some commands to configure the WireGuard interface
if [[ -n $wg_interface ]]; then
    # Store IPs as "<IPv4> <IPv6>"" when they are logged
    ips=$(timeout 10s tail -n +1 -f toverstok_log.txt | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    # Parse IPs: sed output is "<IPv4> <IPv6>" if everything goes correctly
    if [[ -z $ips ]]; then echo "TS_FAIL: could not find virtual IPs in logs"; exit; fi
    ipv4=$(echo $ips | cut -d ' ' -f1) 
    ipv6=$(echo $ips | cut -d ' ' -f2)

    # Add IPs to WireGuard interface
    ip address add $ipv4 dev $wg_interface
    ip address add $ipv6 dev $wg_interface

    fix_wg_interface

    # Extract peer's virtual IP address
    virtual_ipv4=$(wg | grep -Eo "allowed ips: [0-9.]+" | cut -d ' ' -f3)
    virtual_ipv6=$(wg | grep -Eo "allowed ips: (\S+) [0-9a-f:]+" | cut -d ' ' -f4)

    # Try to ping the peer's IPv4 address
    if ! ping -c 1 $virtual_ipv4 &> /dev/null; then
        echo "TS_FAIL: IPv4 ping failed with IP address: ${virtual_ipv4}"
        exit 1
    fi 

    # Try to ping the peer's IPv6 address
    if ! ping -c 1 $virtual_ipv6 &> /dev/null; then
        echo "TS_FAIL: IPv6 ping failed with IP address: ${virtual_ipv6}"
        exit 1
    fi   

    echo "TS_PASS"

    # Sleep for short duration to give other peer time to ping
    sleep 1s
fi