# Changelog

In this file, the test suite features that have been made possible thanks to [funding from NLnet](./README.md#funding) are documented. 


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