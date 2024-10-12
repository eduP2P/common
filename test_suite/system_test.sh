#!/bin/bash

test_idx=$1
test_target=$2
control_pub_key=$3
control_ip=$4
control_port=$5
relay_port=$6
adm_ips=$7
log_lvl=$8
log_dir=$9
repo_dir=${10}
nat_config_str=${11}
ns_config_str=${12}
wg_interface_str=${13}

usage_str="""
Usage: ${0} <TEST INDEX> <CONTROL SERVER PUBLIC KEY> <CONTROL SERVER IP> <CONTROL SERVER PORT> <RELAY SERVER PORT> <IP ADDRESS LIST> <LOG LEVEL> <LOG DIRECTORY> <REPOSITORY DIRECTORY> <NAT CONFIGURATION 1>:<NAT CONFIGURATION 2> <NAMESPACE CONFIGURATION> [WIREGUARD INTERFACE 1]:[WIREGUARD INTERFACE 2]

<IP ADDRESS LIST> is a string of IP addresses separated by a space that may be the destination IP of packets crossing this NAT device, and are necessary to simulate an Address-Dependent Mapping

<LOG LEVEL> should be one of {trace|debug|info} (in order of most to least log messages), but can NOT be info if one if the peers is using userspace WireGuard (then IP of the other peer is not logged)

<NAT CONFIGURATION 1> and <NAT CONFIGURATION 2> specify the type of NAT applied to packets sent by peer 1 and 2 respectively. Both should be a string with the format:
    <NAT MAPPING TYPE>-<NAT FILTERING TYPE>, where both may be one of the following numbers: 
        0 - Endpoint-Independent
        1 - Address-Dependent
        2 - Address and Port-Dependent
Example of a valid NAT configuration: 0-1:1-2

<NAMESPACE CONFIGURATION> specifies the peer and router namespaces to be used in this system test. It should be a string  with the format:
    <PEER 1 NAMESPACE>-<ROUTER 1 NAMESPACE>:<ROUTER 2 NAMESPACE>-<PEER 2 NAMESPACE> for peers in different private networks, or 
    <PEER 1 NAMESPACE>-<ROUTER 1 NAMESPACE>-<PEER 2 NAMESPACE> for peers in the same private network

If [WIREGUARD INTERFACE 1] or [WIREGUARD INTERFACE 2] is not provided, the corresponding peer will use userspace WireGuard"""

# Make sure all arguments have been passed
if [[ $# -ne 13 ]]; then
    echo $usage_str
    exit 1
fi

# Function to validate string against regular expression
function validate_str() {
    str=$1
    regex=$2

    if [[ ! $str =~ $regex ]]; then
        echo $usage_str
        exit 1
    fi
}

# Parse NAT configuration string into individual Mapping and Filtering values
nat_config_regex="^([0-2])-([0-2]):([0-2])-([0-2])$"
validate_str $nat_config_str $nat_config_regex 
nat_map=(${BASH_REMATCH[1]} ${BASH_REMATCH[3]})
nat_filter=(${BASH_REMATCH[2]} ${BASH_REMATCH[4]})

# Parse namespace configuration string into individual peer and router namespaces
ns_config_regex="^([^-]+)-([^:-]+):?([^:-]+)?-([^-]+)$"
validate_str $ns_config_str $ns_config_regex 
n_groups=$((${#BASH_REMATCH[@]} - 1))
peer_ns_list=(${BASH_REMATCH[1]} ${BASH_REMATCH[$n_groups]})
router_ns_list=()

for ((i=2; i<$n_groups; i++)); do
    router_ns_list+=(${BASH_REMATCH[$i]})
done

# Parse WireGuard interfaces string into individual interfaces
wg_interface_regex="^([^:]*):([^:]*)$"
validate_str $wg_interface_str $wg_interface_regex 
wg_interfaces=(${BASH_REMATCH[1]} ${BASH_REMATCH[3]})

# Prepare a string describing this system test
NAT_TYPES_LONG=("Endpoint-Independent" "Address-Dependent" "Address and Port-Dependent")
NAT_TYPES=("EI" "AD" "APD")

function describe_nat() {
    i=$1

    echo "${NAT_TYPES[${nat_map[$i]}]}M-${NAT_TYPES[${nat_filter[$i]}]}F"
}

nat1_description=$(describe_nat 0)
nat2_description=$(describe_nat 1)

test_description="Test $test_idx. ${nat1_description} <-> ${nat2_description}, target=$test_target, result="

# Output test description 
echo -n "$test_description"

# Add log subdirectory for this system test
new_dir="${log_dir}/${test_idx}_${nat1_description}_${nat2_description}"
mkdir $new_dir
log_dir=$new_dir

# Cleanup function called at end of script
function cleanup () {
    # Kill the conntrack processes started by the nat simulation scripts
    conntrack_pids=$(pidof conntrack)
    if [[ -n $conntrack_pids ]]; then sudo kill $conntrack_pids; fi

    # Log final nftables configuration and conntrack list of the routers
    for router_ns in ${router_ns_list[@]}; do
        echo "nftables configuration after test finished:" >> ${log_dir}/$router_ns.txt
        sudo ip netns exec $router_ns nft list ruleset >> ${log_dir}/$router_ns.txt

        echo "conntrack list after test finished:" >> ${log_dir}/$router_ns.txt
        sudo ip netns exec $router_ns conntrack -L &>> ${log_dir}/$router_ns.txt
    done

    # Reset nftables configuration of the routers
    for router_ns in ${router_ns_list[@]}; do
        sudo ip netns exec $router_ns nft flush ruleset
    done
}

trap cleanup EXIT 

# Start NAT simulation on each router
cd ${repo_dir}/test_suite/nat_simulation

for ((i=0; i<${#router_ns_list[@]}; i++)); do
    router_ns=${router_ns_list[$i]}
    sudo ip netns exec $router_ns ./setup_nat_mapping.sh ${router_ns}_pub 10.0.$((i+1)).0/24 ${nat_map[$i]} "${adm_ips}"
    sudo ip netns exec $router_ns ./setup_nat_filtering.sh ${router_ns}_pub 10.0.$((i+1)).0/24 ${nat_filter[$i]} 2>&1 | \
    tee ${log_dir}/$router_ns.txt > /dev/null & # combination of tee and redirect to /dev/null is necessary to avoid weird behaviour caused by redirecting a script run with sudo
done

# Start peers and save their PIDs
cd ${repo_dir}/cmd/toverstok
peer_pids=()

for i in {0..1}; do 
    peer_id="peer$((i+1))"
    peer_ns=${peer_ns_list[$i]}

    sudo ip netns exec $peer_ns ./setup_client.sh $peer_id $control_pub_key $control_ip $control_port $log_lvl ${wg_interfaces[$i]} `# Run script from the peer's isolated namespace` \
    &> >(sed -r "/TS_(PASS|FAIL)/q" > ${log_dir}/$peer_id.txt) & # Use sed to copy STDOUT and STDERR to a log file until the test suite's exit code is found (sed is run in subshell so $! will return the pid of setup_client.sh)

    peer_pids+=($!)
done

# Wait until peer processes are finished
wait ${peer_pids[@]}

# Constants for colored text in output
RED="\033[0;31m"
GREEN="\033[0;32m"
NC="\033[0m" # No color

# Throw error if one of the two peers did not exit with TS_PASS or timed out
for i in {0..1}; do 
    peer_id="peer$((i+1))"
    export LOG_FILE=${log_dir}/$peer_id.txt # Export to use in bash -c
    timeout 15s bash -c 'tail -n 1 -f $LOG_FILE | sed -n "/TS_PASS/q2; /TS_FAIL/q3"' # bash -c is necessary to use timeout with | and still get the right exit codes

    # Branch on exit code of previous command
    case $? in
        0|1) echo -e "${RED}TS_FAIL: error when searching for exit code in logs of peer $peer_id${NC}"; exit 1 ;; # 0 and 1 indicate tail/sed failure
        2) ;; # 2 indicates TS_PASS was found
        3) echo -e "${RED}TS_FAIL: test failed for peer $peer_id; view this peer's logs for more information${NC}"; exit 1 ;; # 3 indicates TS_FAIL was found
        124) echo -e "${RED}TS_FAIL: timeout when searching for exit code in logs of peer $peer_id${NC}"; exit 1 ;; # 124 is default timeout exit code
        *) echo -e "${RED}TS_FAIL: unknown error${NC}"; exit 1 ;;
    esac
done

# Verify whether peers established a direct connection by searching for specific log message in either of the peers' logs
if grep -q "ESTABLISHED direct peer connection" ${log_dir}/peer*; then
    test_result="TS_PASS_DIRECT"
else
    test_result="TS_PASS_RELAY"
fi

# Output test result 
if [[ $test_target != $test_result ]]; then
    echo -e "${RED}$test_result${NC}"
    exit 1
fi

echo -e "${GREEN}$test_result${NC}"