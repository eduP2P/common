
#!/bin/bash

# Usage: ./setup_public.sh <SWITCH IP> <CONTROL SERVER IP> <RELAY SERVER IP>
# This script must be run with root permissions

switch_ip=$1
control_ip=$2
relay_ip=$3

# Turn on loopback interface in namespace to allow pinging from inside namespace
ip link set lo up

# Create TUN interface for switch
sudo ip tuntap add dev switch mode tun
sudo ip link set dev switch up
sudo ip addr add "${switch_ip}/24" dev switch

# Create TUN interface for control server
sudo ip tuntap add dev control mode tun
sudo ip link set dev control up
sudo ip addr add "${control_ip}/24" dev control  

# Create TUN interface for relay server
sudo ip tuntap add dev relay mode tun
sudo ip link set dev relay up
sudo ip addr add "${relay_ip}/24" dev relay