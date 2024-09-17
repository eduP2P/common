#!/bin/bash

# Usage: ./setup_toverstok.sh <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <LOG LEVEL> <WIREGUARD INTERFACE> 
# <LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages)
# <WIREGUARD INTERFACE> is optional, if it is not set eduP2P is configured with userspace WireGuard

control_pub_key=$1
control_ip=$2
control_port=$3
log_lvl=$4
wg_interface=$5

# Create WireGuard interface if wg_interface is set, otherwise set up /dev/net/tun for userspace WireGuard
if [[ -n $wg_interface ]]; then
    ip link add $wg_interface type wireguard
    wg set $wg_interface listen-port 0 # 0 means port is chosen randomly
    ip link set $wg_interface up
else
    mkdir -p /dev/net
    mknod /dev/net/tun c 10 200
    chmod 600 /dev/net/tun
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

function cleanup () {
    # Kill process feeding input into pipe
    kill %1

    # Remove pipe
    rm toverstok_in

    # Terminate toverstok, which remains open with userspace WireGuard
    toverstok_pid=$(pgrep toverstok) && kill $toverstok_pid
}

# Remove pipe and kill background processes when script finishes
trap cleanup EXIT

# Create log file for toverstok
touch toverstok_log.txt

# Generate commands from template and put them in the pipe
while read line; do
    eval $line > toverstok_in
done < commands_template.txt

# If wg_interface is set, eduP2P will print some commands to configure the WireGuard interface
if [[ -n $wg_interface ]]; then
    # Store IPs as "<IPv4> <IPv6>"" when they are logged
    ips=$(timeout 10s tail -n +1 -f toverstok_log.txt | sed -rn "/.*sudo ip address add (\S+) dev ${wg_interface}; sudo ip address add (\S+) dev ${wg_interface}.*/{s//\1 \2/p;q}")

    if [[ -z $ips ]]; then echo "TS_FAIL: could not find own virtual IPs in logs"; exit; fi

    ipv4=$(echo $ips | cut -d ' ' -f1) 
    ipv6=$(echo $ips | cut -d ' ' -f2)

    # Add IPs to WireGuard interface
    ip address add $ipv4 dev $wg_interface
    ip address add $ipv6 dev $wg_interface

    # Sleep for short duration to give toverstok time to update WireGuard interface
    sleep 5s

    # Extract peer's virtual IP address from WireGuard interface
    virtual_ipv4=$(wg | grep -Eo "allowed ips: [0-9.]+" | cut -d ' ' -f3)
    virtual_ipv6=$(wg | grep -Eo "allowed ips: (\S+) [0-9a-f:]+" | cut -d ' ' -f4)
# When using userspace WireGuard, we can skip the configuration but need to extract the virtual IPs from the logs
else
    # Store virtual IPs as "<IPv4> <IPv6>"" when they are logged
    virtual_ips=$(timeout 10s tail -n +1 -f toverstok_log.txt | sed -rn "/.*IPv4:(\S+) IPv6:(\S+).*/{s//\1 \2/p;q}")

    if [[ -z $virtual_ips ]]; then echo "TS_FAIL: could not find peer's virtual IPs in logs"; exit; fi

    virtual_ipv4=$(echo $virtual_ips | cut -d ' ' -f1) 
    virtual_ipv6=$(echo $virtual_ips | cut -d ' ' -f2)
fi

# Try to ping the peer's IPv4 address
if ! ping -c 5 $virtual_ipv4 &> /dev/null; then
    echo "TS_FAIL: IPv4 ping failed with IP address: ${virtual_ipv4}"
    exit 1
fi 

# Try to ping the peer's IPv6 address
if ! ping -c 5 $virtual_ipv6 &> /dev/null; then
    echo "TS_FAIL: IPv6 ping failed with IP address: ${virtual_ipv6}"
    exit 1
fi   

echo "TS_PASS"

# Sleep for short duration to give other peer time to ping
sleep 10s