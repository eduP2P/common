# Changelog

In this file, the test suite features that have been made possible thanks to [funding from NLnet](./README.md#funding) are documented. 

## Double NAT(Oct 21, 2025)
### Added
- Explanation of Double NAT and how it is implemented in the test suite in the [system test documentation](./README.md#double-nat).
- The `-2` flag to [`system_tests.sh`](system_tests.sh) and [`nat_simulation/setup_networks.sh`](nat_simulation/setup_networks.sh), which enables Double NAT.
- New logic in [`system_tests.sh`](system_tests.sh) which decides the expected test result if the peers are behind Double NAT. This logic assumes the NAT behaviour is limited to the 4 NATs described in RFC 3489
- Functionality in [`nat_simulation/setup_router.sh`](nat_simulation/setup_router.sh) to perform different actions depending on which of the two NATs is being set up.
- Report on the system tests results with Double NAT for each combination of two RFC 3489 NATs in the [system test results](./README.md#effect-of-double-nat-on-system-test-results).

### Changed
- The syntax of the NAT and network namespace configurations which are passed as parameters to [`system_test.sh`](system_test.sh), such that a second NAT layer can be specified.

### Fixed
- Small miscellaneous improvements, such as:
  - Less duplicated code for regex validation by placing regular expressions which are used multiple times in [util.sh](util.sh).
  - Fix incorrect abbreviation ADPF -> APDF across documentation and code comments.
  - Add missing command necessary before running parallel system tests in the [system test requirements](./README.md#system-test-specific-requirements).

## NAT IP pooling (June 20, 2025)
### Added
- Explanation of NAT IP pooling and how it is implemented in the test suite in the [system test documentation](./README.md#ip-address-pooling).
- The `-n` flag to [`system_tests.sh`](system_tests.sh) and a positional parameter to [`system_test.sh`](system_test.sh),[`nat_simulation/setup_networks.sh`](nat_simulation/setup_networks.sh) and [`nat_simulation/setup_router.sh`](nat_simulation/setup_router.sh), which specify the amount of IP addresses available to the routers for NAT IP pooling.
- New expected test result `TS_PASS`, which accepts both a direct and relayed connection between the peers. This new result is necessary because for some NAT combinations with IP pooling, it is uncertain whether a direct connection can be established.
- Report on which system tests are affected by IP pooling in the [system test results](./README.md#effect-of-ip-address-pooling-on-system-test-results).

### Changed
- The logic in [`system_tests.sh`](system_tests.sh) which decides the expected test result based on the specified NAT combination of the peers. This logic now uses the new `TS_PASS` result for certain combinations if NAT IP pooling is enabled.
- Branches on the (expected) test result in [`system_test.sh`](system_test.sh) and [`test_client/setup_client.sh`](test_client/setup_client.sh), such that they also take the new `TS_PASS` result into account.
- Older results of the system tests without NAT IP pooling. They now contain a reference to the new results, and their visualization has been changed to align with the new results. 
- The implementation of the NAT mapping & filtering behaviour, respectively found in [`nat_simulation/setup_networks.sh`](nat_simulation/setup_networks.sh) and [`nat_simulation/setup_router.sh`](nat_simulation/setup_router.sh). The implementation of the mapping behaviour now also does NAT IP pooling, and the filtering behaviour had to be adjusted to take the multiple IP addresses into account.

## Parallel system tests (April 11, 2025)

### Added
- [Dockerfile](Dockerfile) that installs all requirements to run the test suite's system tests, clones the repository state at the head of a specified branch, and build the eduP2P client, control server and relay server test binaries.
- Docker Engine as new requirement in [system test requirements](README.md#system-test-specific-requirements).
- `-t` flag in [system_tests.sh](system_tests.sh) to run the system test in parallel with the specified amount of threads. Each thread is a Docker container in which a portion of the system tests is executed.
- SystemTestsParallel job in [test suite CI workflow](../.github/workflows/CI_test_suite.yml) that builds the Dockerfile with caching and runs the system tests with the `-t` flag.
- Explanation of the `-t` flag, motivation for using Docker and reason why parallel system tests currently do not speed up CI runs in the [system test documentation](README.md#system-tests).
- Log level `trace` (most detailed) in [test_client/main.go](test_client/main.go).

### Changed
- Building of eduP2P binaries in [system_tests.sh](system_tests.sh) now only happens when explicitly providing the new `-b` flag to save time when the binaries have already been built (e.g. in Dockerfile or on local machine).
- Optimizations in [test_client/setup_client.sh](test_client/setup_client.sh), such as smaller sleep durations to make the system tests run faster.

### Fixed
- Bug in [visualize_performance_tests.py](visualize_performance_tests.py): quotation marks inside a format string caused error in some Python versions.
- Handshake error between eduP2P peers in the system tests caused by both peers initializing a handshake simultaneously. Fixed by desynchronizing the peers with a conditional sleep in [test_client/setup_client.sh](test_client/setup_client.sh). 

## Repeated performance tests (March 7, 2025)

### Added
- `-r` flag in [performance_tests.sh](performance_tests.sh) to repeat the same performance test multiple times and aggregate the results of each repetition by taking their average.
- Explanation of the `-r` flag in the [performance test documentation](./README.md#performance-tests).
- Report on how aggregating the performance test results can improve their reliability in the [performance test results](./README.md#consistency-of-results).

## Simulating network delay (March 4, 2025)

### Added
- `-d` flag in [system_tests.sh](system_tests.sh) to add artificial network delay in the system tests.
- New value `delay` for the `-k` flag in [performance_tests.sh](performance_tests.sh) to add variable artificial network delay during the performance tests.
- Explanation of the delay variable in the [performance test documentation](./README.md#performance-tests).
- Report on how the delay affects eduP2P network performance in the [performance test results](./README.md#results-with-varying-one-way-delay).