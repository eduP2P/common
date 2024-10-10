#!/bin/bash

control_port=$1
relay_port=$2
log_lvl=$3
nat_config_str=$4
wg_interface_str=$5

# Make sure all arguments have been passed, and nat_config_str has the correct format
if [[ $# -ne 5 || ! ($nat_config_str =~ ^[0-2]-[0-2]:[0-2]-[0-2]$)]]; then
    echo """
Usage: ${0} <CONTROL SERVER PORT> <RELAY SERVER PORT> <LOG LEVEL> <NAT CONFIGURATION 1>:<NAT CONFIGURATION 2> [WIREGUARD INTERFACE 1]:[WIREGUARD INTERFACE 2]

<LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)

<NAT CONFIGURATION 1> and <NAT CONFIGURATION 2> specify the type of NAT applied to packets sent by peer 1 and 2 respectively. Both should be a string with the format:
    <NAT MAPPING TYPE>-<NAT FILTERING TYPE>, where both may be one of the following numbers: 
        0 - Endpoint-Independent
        1 - Address-Dependent
        2 - Address and Port-Dependent
Example of a valid NAT configuration: 0-1:1-2

If [WIREGUARD INTERFACE 1] or [WIREGUARD INTERFACE 2] is not provided, the corresponding peer will use userspace WireGuard"""
    exit 1
fi

NAT_TYPES=("Endpoint-Independent" "Address-Dependent" "Address and Port-Dependent")
nat_map=()
nat_filter=()

echo "Starting system test between two peers:"

for i in {1..2}; do
    nat_config=$(echo $nat_config_str | cut -d ':' -f$i)
    nat_map[$i]=$(echo $nat_config | cut -d '-' -f1)
    nat_filter[$i]=$(echo $nat_config | cut -d '-' -f2)

    echo "  - Peer ${i} is behind a NAT device with an ${NAT_TYPES[${nat_map[$i]}]} Mapping and ${NAT_TYPES[${nat_filter[$i]}]} Filtering"
done

wg_interfaces=()

for i in {1..2}; do
    wg_interfaces[$i]=$(echo $wg_interface_str | cut -d ':' -f$i)
done

# Store present working directory and cmd folder for later use
pwd=$(pwd)

# Get IP of control and relay servers which reside in the public namespace
control_ip=$(sudo ip netns exec public ip addr show control | grep -Eo "inet [0-9.]+" | cut -d ' ' -f2)
relay_ip=$(sudo ip netns exec public ip addr show relay | grep -Eo "inet [0-9.]+" | cut -d ' ' -f2)

# Create directory to store logs
timestamp=$(date +"%Y-%m-%dT%H_%M_%S")
log_dir=logs/${timestamp}
log_dir_abs=${pwd}/logs/${timestamp}
mkdir -p ${log_dir}
echo "Logging to ${log_dir}"

# Store PIDs of background processes that must be killed when script exits
background_pids=()

function cleanup () {
    # Kill the two servers and client setup scripts if they have already been started by the script
    control_pid=$(pgrep control_server) && sudo kill $control_pid
    relay_pid=$(pgrep relay_server) && sudo kill $relay_pid

    # Kill the conntrack processes started by the nat simulation scripts, and ignore their output while terminating
    conntrack_pids=$(pidof conntrack)

    if [[ -n $conntrack_pids ]]; then
        sudo kill $(pidof conntrack)
        wait $(pidof conntrack) &> /dev/null
    fi

    # Reset nftables configuration of the routers
    for i in {1..2}; do
        sudo ip netns exec router${i} nft flush ruleset
    done
}

# Run cleanup when script exits
trap cleanup EXIT 

# Extract public key of control server
cd ../cmd/control_server
control_pub_key=$(./setup_control_server.sh $control_ip $control_port)

# If key variable is empty, server did not start successfully
if [[ -z $control_pub_key ]]; then
    echo "TS_FAIL: error when starting control server with IP ${control_ip} and port ${control_port}"
    exit 1
fi

# Extract public key of relay server
cd ../relay_server
relay_pub_key=$(./setup_relay_server.sh $relay_ip $relay_port)

# If key variable is empty, server did not start successfully
if [[ -z $relay_pub_key ]]; then
    echo "TS_FAIL: error when starting relay server with IP ${relay_ip} and port ${relay_port}"
    exit 1
fi

# Add relay server to control server config
cd ../control_server
sudo python3 configure_json.py $relay_pub_key $relay_ip $relay_port

# Run servers as background processes
echo "Starting servers in public namespace"
./start_control_server.sh $control_ip $control_port 2>&1 | tee ${log_dir_abs}/control.txt > /dev/null &
cd ../relay_server
./start_relay_server.sh $relay_ip $relay_port 2>&1 | tee ${log_dir_abs}/relay.txt > /dev/null &

# Use above image to run two peers and store their container ids
echo "Starting toverstok clients in network namespaces to simulate two isolated peers"
cd ../toverstok

# Build toverstok client
go build -o toverstok *.go &> /dev/null

for i in {1..2}; do
    router_ns="router${i}"
    peer_ns="private${i}_peer1"
    log_file=${log_dir_abs}/peer${i}.txt

    cd $pwd/nat_simulation
    sudo ip netns exec ${router_ns} ./setup_nat.sh ${router_ns}_pub 10.0.${i}.0/24 ${nat_map[$i]} ${nat_filter[$i]} $pwd/adm_ips.txt 2>&1 | tee ${log_dir_abs}/${router_ns}.txt > /dev/null &
    cd $pwd/../cmd/toverstok

    sudo ip netns exec ${peer_ns} ./setup_client.sh $i $control_pub_key $control_ip $control_port $log_lvl ${wg_interfaces[$i]} `# Run script from the peer's isolated namespace` \
    2>&1 | `# Combine STDERR and STDOUT into one output stream` \
    sed -r "/TS_(PASS|FAIL)/q" > $log_file & # Use sed to copy the combined output stream to the specified log file, until the test suite's exit code is found
done

# Throw error if one of the two peers did not exit with TS_PASS or timed out
for i in {1..2}; do 
    export LOG_FILE=${log_dir_abs}/peer${i}.txt # Export to use in bash -c
    timeout 60s bash -c 'tail -n +1 -f $LOG_FILE | sed -n "/TS_PASS/q2; /TS_FAIL/q3"' # bash -c is necessary to use timeout with | and still get the right exit codes
    
    # Branch on exit code of previous command
    case $? in
        0|1) echo "TS_FAIL: error when searching for exit code in logs of peer ${i}"; exit 1 ;; # 0 and 1 indicate tail/sed failure
        2) echo "Peer #${i} success" ;; # 2 indicates TS_PASS was found
        3) echo "TS_FAIL: test failed for peer ${i}; view this peer's logs for more information"; exit 1 ;; # 3 indicates TS_FAIL was found
        124) echo "TS_FAIL: timeout when searching for exit code in logs of peer ${i}"; exit 1 ;; # 124 is default timeout exit code
        *) echo "TS_FAIL: unknown error"; exit 1 ;;
    esac
done

# Verify whether peers established a direct connection by searching for specific log message in either of the peers' logs
if grep -q "ESTABLISHED direct peer connection" ${log_dir_abs}/peer*; then
    echo "TS_PASS: direct connection established"
else
    echo "TS_PASS: direct connection failed, used relay server as fallback"
fi