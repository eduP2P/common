#!/bin/bash

# Usage: ./system_test.sh <CONTROL SERVER PORT> <RELAY SERVER PORT> <LOG LEVEL> <WG_INTERFACE_1>:<WG_INTERFACE_2>
# <LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)
# Providing an empty string for WG_INTERFACE_{1|2} will make that peer use userspace WireGuard

control_port=$1
relay_port=$2
log_lvl=$3
wg_interface_str=$4

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

function cleanup () {
    # Kill the two servers and client setup scripts if they have already been started by the script
    control_pid=$(pgrep control_server) && sudo kill $control_pid
    relay_pid=$(pgrep relay_server) && sudo kill $relay_pid
}

trap cleanup EXIT # Run cleanup when script exits

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
    log_file=${log_dir_abs}/peer${i}.txt
    peer_ns="private${i}_peer1"

    sudo ip netns exec ${peer_ns} ./setup_client.sh $i $control_pub_key $control_ip $control_port $log_lvl ${wg_interfaces[$i]} `# Run script from the peer's isolated namespace` \
    2>&1 | `# Combine STDERR and STDOUT into one output stream` \
    sed -r "/TS_(PASS|FAIL)/q" > $log_file & # Use sed to copy the combined output stream to the specified log file, until the test suite's exit code is found
done

# Throw error if one of the two peers did not exit with TS_PASS or timed out
for i in {1..2}; do 
    export LOG_FILE=${log_dir_abs}/peer${i}.txt # Export to use in bash -c
    timeout 60 bash -c 'tail -n +1 -f $LOG_FILE | sed -rn "/TS_PASS/q2; /TS_FAIL/q3"' # bash -c is necessary to use timeout with |
    
    # Branch on exit code of previous command
    case $? in
        0|1) echo "TS_FAIL: error when searching for exit code in logs of peer ${i}"; exit 1 ;; # 0 and 1 indicate docker logs/sed failure
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