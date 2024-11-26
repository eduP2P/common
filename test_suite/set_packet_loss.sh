#!/bin/bash

usage_str="""
Usage: ${0} <PACKET LOSS PERCENTAGE>

<PACKET LOSS PERCENTAGE> should be a real number in the interval [0, 100)"""

packet_loss=$1

# Make sure packet_loss is a real number, and get the amount of decimal digits
real_regex="^[0-9]+[.]?([0-9]+)?$"

if [[ $# -ne 1 || ! ( $packet_loss =~ $real_regex) ]]; then
    echo $usage_str
    exit 1
fi

n_decimals=${#BASH_REMATCH[1]}

# Make sure packet_loss is in the interval [0, 100)
in_interval=$(echo "$packet_loss >= 0 && $packet_loss < 100" | bc) # 1=true, 0=false

if [[ $in_interval -eq 0 ]]; then
    echo $usage_str
    exit 1
fi

# Multiply packet_loss by n_decimals to turn it into an integer
packet_loss_int=$(echo "$packet_loss * 10^$n_decimals" | bc | cut -d '.' -f1) 

# Remove any packet loss rule currently in the filter table's forward chain 
sudo ip netns exec public nft flush chain inet filter forward

# Add a new rule that drops packet_loss_int/modulus packets, equivalent to packet_loss% of packets
modulus=$(( 100 * 10 ** $n_decimals ))
sudo ip netns exec public nft add rule inet filter forward numgen random mod $modulus \< $packet_loss_int counter drop