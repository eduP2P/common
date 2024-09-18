#!/bin/bash

# Usage: ./setup.sh <CONTROL SERVER IP> <CONTROL SERVER PORT> <RELAY SERVER IP> <RELAY SERVER PORT> <LOG LEVEL> <WG_INTERFACE_1>:<WG_INTERFACE_2>
# <LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)
# Providing an empty string for WG_INTERFACE_{1|2} will make that peer use userspace WireGuard

control_ip=$1
control_port=$2
relay_ip=$3
relay_port=$4
log_lvl=$5
wg_interfaces=()

for i in {1..2}; do
    wg_interfaces[$i]=$(echo $6 | cut -d ':' -f$i)
done

# Create directory to store logs
timestamp=$(date +"%Y-%m-%dT%H_%M_%S")
mkdir -p logs/${timestamp}
echo "Logging to logs/${timestamp}"

# Store present working directory and cmd folder for later use
pwd=$(pwd)

function cleanup () {
    # Kill the two servers if they have already been started by the script
    control_pid=$(pgrep control_server) && kill $control_pid
    relay_pid=$(pgrep relay_server) && kill $relay_pid

    # Save logs of all containers
    cd $pwd

    for i in {1..2}; do
        docker logs ${container_ids[$i]} > logs/${timestamp}/peer${i}.txt
    done
}

trap cleanup EXIT # Run cleanup when script exits

# Extract public key of control server
cd ../cmd/control_server
control_pub_key=$(./setup_control_server.sh $control_port)

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
python3 configure_json.py $relay_pub_key $relay_ip $relay_port

# Run servers as background processes
echo "Starting servers"
(./start_control_server.sh $control_port &> $pwd/logs/${timestamp}/control.txt &)
cd ../relay_server
(./start_relay_server.sh $relay_ip $relay_port &> $pwd/logs/${timestamp}/relay.txt &)

# Create docker image to simulate an isolated peer
echo "Built docker image with ID: $(docker build -qt peer ../toverstok)"

# Use above image to run two peers and store their container ids
container_ids=()

echo "Starting containers to simulate two peers"
for i in {1..2}; do
    container_ids[$i]=$(
        docker run \
        --network peer$i `# The name of the network used by the container, should have been created before executing this script` \
        --cap-add=NET_ADMIN `# Required to create and configure WireGuard interface in container` \
        --sysctl net.ipv6.conf.default.disable_ipv6=0 `# Allow IPv6 in this WireGuard interface` \
        -dt peer `# Run 'peer' image in detached mode, which returns the container ID` \
        $control_pub_key $control_ip $control_port $log_lvl ${wg_interfaces[$i]} `# Arguments of setup_client.sh, the entrypoint script of the docker image`
    )
    echo "Peer #${i} container ID: ${container_ids[$i]}"
done

# Throw error if one of the two peers did not exit with TS_PASS or timed out
for i in {1..2}; do 
    export CID=${container_ids[$i]} # Export to use in bash -c
    timeout 60 bash -c 'docker logs -f $CID | sed -rn "/TS_PASS/q2; /TS_FAIL/q3"' # bash -c is necessary to use timeout with |
    
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