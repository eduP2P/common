#!/usr/bin/env bash

usage_str="""
Usage: ${0} [OPTIONAL ARGUMENTS]

This script runs system tests between two eduP2P peers sequentially

The following options determine the type of tests run:
    -e
        Run extended connectivity tests (all combinations of RFC 4787 NAT mapping and filtering behaviour)
    -f <file>
        Run custom tests from an existing file. One test should be specified on a single line, and this line should be a call to the run_system_test function found in this script
    -p
        Run some examples of the test suite's performance tests

If none of these options are provided, a shortened version of the connectivity tests is run (all combinations of RFC 3489 NATs)

The following options can be used to configure additional parameters during the tests:
    -c <packet loss>
        Simulate packet loss by making the central network switch randomly drop a percentage of packets
        This percentage should be provided as a real number in the interval [0, 100)
    -d <delay>
        Add delay to packets transmitted by the eduP2P clients, control server and relay server
        The delay should be provided as an integer that represents the one-way delay in milliseconds
    -l <trace|debug|info|warn|error>
        Specifies the log level used in the eduP2P client of the two peers
        If one of the peers uses userspace WireGuard, the log level trace/debug must be used, since the other peer's IP address is not logged otherwise
    -L <log directory>
        Specifies the alphanumeric name of the directory inside system_test_logs/ where the test logs will be stored
        If this argument is not provided, the directory name is the current timestamp
    -t <number of threads between 2 and 8>
        Run the system tests in parallel with the specified number of threads.
    -b
        Build the client, control server and relay server binaries before running the tests"""

# Use functions and constants from util.sh
. ./util.sh

# Default log level
log_lvl="debug"

# Validate optional arguments
while getopts ":c:d:ef:l:L:t:bph" opt; do
    case $opt in
        c)  
            connectivity=true
            packet_loss=$OPTARG

            # Make sure packet_loss is a real number
            real_regex="^[0-9]+[.]?([0-9]+)?$"
            validate_str $packet_loss $real_regex

            # Make sure packet loss is in the interval [0, 100)
            in_interval=$(echo "$packet_loss >= 0 && $packet_loss < 100" | bc) # 1=true, 0=false

            if [[ $in_interval -eq 0 ]]; then
                exit_with_error "packet loss argument is not in the interval [0, 100)"
            fi
            ;;
        d)
            delay=$OPTARG

            # Make sure delay is an integer
            int_regex="^[0-9]+$"
            validate_str $delay $int_regex
            ;;
        e)
            extended=true
            ;;
        f)
            file=$OPTARG

            # Make sure file exists
            if [[ ! -f $file ]]; then
                exit_with_error "$file does not exist"
            fi
            ;;
        l)  
            log_lvl=$OPTARG

            log_lvl_regex="^trace|debug|info|warn|error?$"
            validate_str $log_lvl $log_lvl_regex
            ;;
        L)
            alphanum_regex="^[a-zA-Z0-9]+$"
            validate_str $OPTARG $alphanum_regex
            log_dir_rel=system_test_logs/$OPTARG
            ;;
        t)
            n_threads=$OPTARG

            # Make sure n_threads is an integer between 2 and 8
            threads_regex="^[2-8]$"
            validate_str $n_threads $int_regex
            ;;
        b)
            build=true
            ;;
        p)
            performance=true
            ;;
        h) 
            echo "$usage_str"
            exit 0
            ;;
        *)
            exit_with_error "invalid option"
            ;;
    esac
done

# Store repository's root directory for later use
repo_dir=$(cd ..; pwd)

function create_log_dir() {
    if [[ -z $log_dir_rel ]]; then
        timestamp=$(date +"%Y-%m-%dT%H_%M_%S")
        log_dir_rel=system_test_logs/${timestamp} # Relative path for pretty printing
    fi

    log_dir=${repo_dir}/test_suite/${log_dir_rel} # Absolute path for use in scripts running from different directories
    mkdir -p ${log_dir}
    echo "Logging to ${log_dir_rel}"
}

create_log_dir

function cleanup () {
    # Kill the two servers if they have already been started by the script
    sudo pkill control_server
    sudo pkill relay_server

    # Kill the currently running system test if it is still running
    sudo kill $test_pid &> /dev/null
}

function build_go() {
    for binary in test_client control_server relay_server; do
        binary_dir="${repo_dir}/test_suite/$binary"
        go build -o "${binary_dir}/$binary" ${binary_dir}/*.go &> /dev/null
    done
}

function setup_networks() {
    cd nat_simulation/
    adm_ips=$(sudo ./setup_networks.sh) # setup_networks.sh returns an array of IPs used by hosts in the network simulation setup, this list is needed to simulate a NAT device with an Address-Dependent Mapping
}

function extract_server_pub_key() {
    server_type=$1 # control_server or relay_server
    ip=$2
    port=$3

    cd ${repo_dir}/test_suite/$server_type
    pub_key=$(eval "./setup_$server_type.sh $ip $port")

    # Throw error if server did not start successfully
    if [[ $? -ne 0 ]]; then
        exit 1
    fi

    echo $pub_key
}

function start_server() {
    server_type=$1 # control_server or relay_server
    ip=$2
    port=$3

    cd ${repo_dir}/test_suite/$server_type

    # Combination of tee and redirect to /dev/null is necessary to avoid weird behaviour caused by redirecting a script run with sudo
    eval "./start_$server_type.sh $ip $port 2>&1 | tee ${log_dir}/$server_type.txt > /dev/null &" 
}

function setup_servers() {
    # Get IP of control and relay servers
    control_ip=$(extract_ipv4 control control)
    relay_ip=$(extract_ipv4 relay relay)

    # Extract servers' public keys
    pub_key_regex="^[0-9a-f]+$"
    control_pub_key=$(extract_server_pub_key "control_server" $control_ip $control_port)

    if [[ $? -ne 0 ]]; then
        echo -e "${RED}Error when starting control server with IP $control_ip and port $control_port${NC}"
        exit 1
    fi

    relay_pub_key=$(extract_server_pub_key "relay_server" $relay_ip $relay_port)

    if [[ $? -ne 0 ]]; then
        echo -e "${RED}Error when starting relay server with IP $relay_ip and port $relay_port${NC}"
        exit 1
    fi

    # Add relay server to control server config
    cd ${repo_dir}/test_suite/control_server
    sudo chmod 666 control.json # Make config file accessible without sudo
    python3 configure_json.py $relay_pub_key $relay_ip $relay_port

    start_server "control_server" $control_ip $control_port
    start_server "relay_server" $relay_ip $relay_port
}

function sequential_setup() {
    # Run cleanup when script exits
    trap cleanup EXIT

    # Go build binaries unless -b flag was specified
    if [[ $build == true ]]; then
        echo "Building binaries..."
        build_go
    else
        echo "Skipped building binaries"
    fi

    setup_networks

    # Choose ports for the control and relay servers, then start them
    control_port=9999
    relay_port=3340
    echo "Setting up servers"
    setup_servers

    cd $repo_dir/test_suite

    if [[ -n $packet_loss ]]; then
        sudo ./set_packet_loss.sh $packet_loss
    fi

    if [[ -n $delay ]]; then
        sudo ./set_delay.sh $delay
    fi

    # Test counters
    n_tests=0
    n_failed=0
}

function parallel_setup() {
    echo """
Dividing the system tests among $n_threads threads. The output of each thread can be found in the logs.
"""

    # The current system tests command will be run in parallel docker containers with a few modifications:
    system_test_opts=$(echo $@ | sed -r -e "s/-f \S+//"   `# Potential -f flag is removed, as each docker container will be assigned a file containing a subset of the current system tests` \
                                        -e "s/-t [2-8]//") # -t flag is removed, since each docker container will run the tests in parallel`

    # Tests will be assigned to the containers in a round-robin manner, so we keep track of the current thread 
    current_thread=0

    # Arrays of length n_threads representing number of assigned and completed tests per thread, initialized to 0
    assigned=()
    completed=()

    for i in $(seq 1 $n_threads); do
        # Initialize above arrays to 0
        assigned+=(0)
        completed+=(0)

        # Create a directory and file for each thread to store the system test logs and commands
        mkdir $log_dir/thread$i
        touch $log_dir/thread$i/tests.txt
    done
}

# Check if -t flag was specified
if [[ -n $n_threads ]]; then
    parallel_setup
else
    sequential_setup
fi

# Log messages that should not be printed when the tests are run in parallel
function log_sequential() {
    msg=$1

    if [[ -z $n_threads ]]; then
        echo -e $msg
    fi
}

# Usage: run_system_test [optional arguments of system_test.sh] <first 4 positional parameters of system_test.sh>
function run_system_test() {
    if [[ -n $n_threads ]]; then # Save the system test to the file corresponding to the current thread
        current_thread_dir="thread$((current_thread+1))"
        echo "run_system_test $@" >> $log_dir/$current_thread_dir/tests.txt
        let "assigned[$current_thread]++"
        let "current_thread = (current_thread+1) % n_threads"
    else # Run the system test now
        let "n_tests++"

        # Run in background and wait for test to finish to allow for interrupting from the terminal
        ./system_test.sh $@ $n_tests $control_pub_key $control_ip $control_port "$adm_ips" $log_lvl $log_dir $repo_dir &
        test_pid=$!
        wait $test_pid

        if [[ $? -ne 0 ]]; then
            let "n_failed++"
        fi
    fi
}

function connectivity_test_logic() {
    ns_config=$1
    wg_config=$2
    nat1_mapping=$3
    nat1_filter=$4
    nat2_mapping=$5
    nat2_filter=$6

    # Determine expected test result
    if [[ $nat1_filter -eq 0 || $nat2_filter -eq 0 ]]; then
        # An EIF NAT always lets the peer's pings through
        test_target="TS_PASS_DIRECT"
    elif [[ $nat1_mapping -eq 0 && $nat2_mapping -eq 0 ]]; then
        # Two peers behind EIM NATs send pings to each other from their own STUN endpoint, to the other's STUN endpoint
        # After sending one ping, the subsequent incoming pings from the peer's STUN endpoint will be accepted, regardless of the filtering behaviour
        test_target="TS_PASS_DIRECT"
    elif [[ $nat1_mapping -eq 0 && $nat1_filter -eq 1 || $nat2_mapping -eq 0 && $nat2_filter -eq 1 ]]; then
        # An EIF-ADF NAT will always let the peer's pings through after sending its first ping
        # This is not a general property of EIM-ADF NATs, but holds in this test suite because each NAT only has one IP address
        test_target="TS_PASS_DIRECT"
    else
        test_target="TS_PASS_RELAY"
    fi

    # Skip symmetrical cases
    if [[ $nat2_mapping -gt $nat1_mapping || $nat2_mapping -eq $nat1_mapping && $nat2_filter -ge $nat1_filter ]]; then 
        nat1=$nat1_mapping-$nat1_filter
        nat2=$nat2_mapping-$nat2_filter

        # Only test RFC 3489 NATs unless the extended flag was set
        if [[ ( ${rfc_3489_nats[*]} =~ $nat1 && ${rfc_3489_nats[*]} =~ $nat2 ) || $extended == true ]]; then
            nat_config=$nat1:$nat2
            run_system_test $test_target $ns_config $nat_config $wg_config
        fi
    fi
}

if [[ $performance == true ]]; then
    log_sequential "\nPerformance tests (without NAT)"
    run_system_test -k bitrate -v 100,200,300,400,500 -d 3 -b both TS_PASS_DIRECT router1-router2 : wg0:wg0
elif [[ -n $file ]]; then
    echo -e "\nTests from file: $file"
    
    # Read line by line from $file (also last line which may not end with a newline, but still contain a command)
    while IFS= read -r test_cmd || [ -n "$test_cmd" ]; do
        eval $test_cmd
    done < $file
else
    rfc_3489_nats=("0-0" "0-1" "0-2" "2-2")

    log_sequential """
Starting connectivity tests between two peers (possibly) behind NATs with various combinations of mapping and filtering behaviour:
    - Endpoint-Independent Mapping/Filtering (EIM/EIF)
    - Address-Dependent Mapping/Filtering (ADM/ADF)
    - Address and Port-Dependent Mapping/Filtering (ADPM/ADPF)"""

    log_sequential "\nTests with one peer behind a NAT"
    for nat_mapping in {0..2}; do
        for nat_filter in {0..2}; do
            nat=$nat_mapping-$nat_filter

            # Only test RFC 3489 NATs unless the extended flag was set
            if [[ ${rfc_3489_nats[*]} =~ $nat || $extended == true ]]; then
                run_system_test TS_PASS_DIRECT private1_peer1-router1:router2 $nat: wg0:
            fi
        done
    done

    log_sequential "\nTests with both peers behind a NAT"
    for nat1_mapping in {0..2}; do
        for nat1_filter in {0..2}; do
            for nat2_mapping in {0..2}; do
                for nat2_filter in {0..2}; do
                    connectivity_test_logic private1_peer1-router1:router2-private2_peer1 wg0: $nat1_mapping $nat1_filter $nat2_mapping $nat2_filter
                done
            done
        done
    done

    log_sequential "\nTest hairpinning"
    for nat_mapping in {0..2}; do
        for nat_filter in {0..2}; do
            nat=$nat_mapping-$nat_filter

            # Only test RFC 3489 NATs unless the extended flag was set
            if [[ ${rfc_3489_nats[*]} =~ $nat || $extended == true ]]; then
                run_system_test TS_PASS_DIRECT private1_peer1-router1-private1_peer2 $nat: wg0:
            fi
        done
    done
fi

function print_summary() {
    if [[ $n_failed -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
    else
        echo -e "${RED}$n_failed/$n_tests tests failed!${NC}"
        exit 1
    fi
}

if [[ -n $n_threads ]]; then
    # Keep track of docker container IDs
    container_ids=()

    docker_log_dir="/go/common/test_suite/system_test_logs"

    for i in $(seq 1 $n_threads); do
        thread="thread$i"
        container_id=$(docker run \
                          --network=host `# Host driver gives faster curl connectivity check` \
                          --cap-add CAP_SYS_ADMIN --cap-add NET_ADMIN --security-opt apparmor=unconfined --device /dev/net/tun:/dev/net/tun `# Permissions required to create network setup` \
                          --mount type=bind,src=$log_dir/$thread,dst=$docker_log_dir/$thread `# Bind logs inside docker container to the corresponding thread on the host` \
                          -dt system_tests -f $docker_log_dir/$thread/tests.txt -L $thread `# Run tests from this thread's file and store the logs in the mounted directory` \
                                           $system_test_opts) # Copy the remaining options from the current system tests command
        container_ids+=($container_id)
    done

    exit_codes=$(docker wait ${container_ids[@]}) # Each exit code represents the amount of failed tests in the corresponding container

    # Replace space delimiters by + and pipe into calculator
    n_failed=$(echo $exit_codes | tr " " + | bc)
    n_tests=$(echo ${assigned[@]} | tr " " + | bc)


    # Log the containers' output
    for i in $(seq 1 $n_threads); do
        thread="thread$i"
        id=${container_ids[$((i-1))]}
        docker logs $id > $log_dir/$thread/cmd_output.txt
    done

    docker rm ${container_ids[@]} > /dev/null
else
    # Create graphs for performance tests, if any were included
    python3 visualize_performance_tests.py $log_dir
fi

print_summary
exit $n_failed