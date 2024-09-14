#!/bin/bash

# Usage: ./setup.sh <CONTROL SERVER IP> <CONTROL SERVER PORT> <RELAY SERVER IP> <RELAY SERVER PORT> <WIREGUARD INTERFACE>
# <WIREGUARD INTERFACE> is optional, if it is not set eduP2P is configured with userspace WireGuard

control_ip=$1
control_port=$2
relay_ip=$3
relay_port=$4
wg_interface=$5

# Create directory to store logs
timestamp=$(date +"%Y-%m-%dT%H_%M_%S")
mkdir -p logs/${timestamp}
echo "Logging to logs/${timestamp}"

# Store present working directory and cmd folder for later use
pwd=$(pwd)

function cleanup () {
    # Kill the two servers created by the script
    kill $(pgrep control_server) $(pgrep relay_server)

    # Save logs of all containers
    cd $pwd

    for id in "${container_ids[@]}"; do
        docker logs $id > logs/${timestamp}/container_${id}.txt
    done
}

trap cleanup EXIT # Run cleanup when script exits

# Extract public key of control server
cd ../cmd/control_server
control_pub_key=$(./setup_control_server.sh $control_ip $control_port)

# Extract public key of relay server
cd ../relay_server
relay_pub_key=$(./setup_relay_server.sh $relay_ip $relay_port)

# Add relay server to control server config
cd ../control_server
python3 configure_json.py $relay_pub_key $relay_ip $relay_port

# Run servers as background processes
echo "Starting servers"
(./start_control_server.sh $control_port &> $pwd/logs/${timestamp}/control.txt &)
cd ../relay_server
(./start_relay_server.sh $relay_ip $relay_port &> $pwd/logs/${timestamp}/relay.txt &)

# Build docker container to simulate two isolated peers
echo "Built docker container with image $(docker build -qt peer ../toverstok)"

# Run two peers and store their container ids
container_ids=()

echo "Starting containers to simulate two peers"
for i in {1..2}; do
    container_ids[$i]=$(docker run --cap-add=NET_ADMIN -dt peer $control_pub_key $control_ip $control_port $wg_interface)
done

# Throw error if one of the two peers did not exit with TS_PASS or timed out
for i in {1..2}; do 
    export CID=${container_ids[$i]} # Export to use in bash -c
    timeout 20 bash -c 'docker logs -f $CID | sed -rn "/TS_PASS/q2; /TS_FAIL/q3"' # bash -c is necessary to use timeout with |
    
    # Branch on exit code of previous command
    case $? in
        0|1) echo "TS_FAIL: error when searching for exit code in docker logs of peer ${i} with container id ${container_ids[$i]}"; exit 1 ;; # 0 and 1 indicate docker logs/sed failure
        2) echo "Peer #${i} success" ;; # 2 indicates TS_SUCCESS was found
        3) echo "TS_FAIL: test failed for peer ${i} with container id ${container_ids[$i]}; view this container's logs for more information"; exit 1 ;; # 3 indicates TS_FAIL was found
        124) echo "TS_FAIL: timeout when searching for exit code in docker logs of peer ${i} with container id ${container_ids[$i]}"; exit 1 ;; # 124 is default timeout exit code
        *) echo "TS_FAIL: unknown error"; exit 1 ;;
    esac
done

echo "TS_PASS"