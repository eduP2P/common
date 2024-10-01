# eduP2P Test Suite

## Table of Contents

1.  [Overview](#overview)
2.  [Requirements](#requirements)
3.  [System Tests](#system-tests)
4.  [Integration Tests](#integration-tests)
5.  [Performance Tests](#performance-tests)
6.  [Test Results](#test-results)
7.  [Network Address Translation](#network-address-translation)
8.  [Bibliography](#bibliography)

## Overview

This test suite verifies whether two clients running the eduP2P
prototype can successfully establish a connection under various
conditions involving Network Address Translation (NAT). Continuous
Integration (CI) is used to automatically run the tests when the
repository is updated. CI is implemented using GitHub Workflows, and the
workflow running the tests can be found
[here](.github/workflows/go.yml).

The test suite contains three types of tests:

1.  System tests to verify the functionality of the whole system.
2.  Integration tests to verify the functionality of smaller parts of
    the system.
3.  Performance tests to measure metrics such as the delay, jitter and
    throughput of the peer-to-peer connection.

## Requirements

The full test suite is known to work on Ubuntu 22.04 and Ubuntu 24.04,
but will probably work on any Linux installation with a bash shell.
Furthermore, any machine running Go version 1.22+ should be able to run
the integration tests. The following software needs to be installed
before the full test suite can be run:

-   Go version 1.22+, which can be installed
    [here](https://go.dev/doc/install) and is necessary to build eduP2P.

### System test-specific requirements

The system tests assume the presence of a simulated multi-network setup
in order to test eduP2P in scenarios involving NAT. This setup is
created by running [a script in the nat\_simulation
subdirectory](nat_simulation/setup_networks.sh) with root privileges,
e.g. by using sudo:

    sudo ./setup_networks.sh 2 1

This script creates network namespaces to simulate isolated networks.
The system tests execute commands in these namespaces, which also
requires root privileges. Therefore, the system tests contain some
commands run with sudo, and running the system tests may result in being
prompted to enter your password.

Furthermore, the system tests require a few command-line tools to be
installed. The list of tools is found in
[system\_test\_requirements.txt](system_test_requirements.txt), and can
be installed by running the following command:

    xargs -a system_test_requirements.txt sudo apt-get install

## System Tests

In these tests, two clients attempt to establish a peer-to-peer
connection using eduP2P. When these tests are executed via GitHub
workflows, the test results can be found in the output of the ‘test’ job
under the step ‘System tests’, and the logs can be downloaded under the
‘Artifacts’ tab. The system tests can also be executed manually with
[this script](setup.sh).

The system tests specifically verify whether the eduP2P peers are able
to establish a connection when NAT is involved. To do so, the local host
running the test suite must simulate a network setup where peers are
located in different private networks, and their routers which act as
the gateway to the public network apply NAT. This simulated setup is
described in the next section.

### Network Simulation Setup

In its most basic form, the eduP2P test suite simulates the following
network setup:

![](./nat_simulation/network_setup.png)

The setup contains two private networks, with subnets `10.0.1.0/24` and
`10.0.2.0/24` respectively, each containing one peer. The routers of the
private networks have a public and private IP. The private IP is part of
the private subnet, and its host part is `254`. The public IPs of router
1 and router 2 are `192.168.1.254` and `192.168.2.254`, respectively.
These routers apply NAT by translating the source IP of outgoing packets
from the private network to the router’s public IP, and by translating
the destination IP of incoming packets back to the corresponding private
host’s address.

There is a network switch with IP address `192.168.0.254` in between the
routers, allowing them to communicate. This network switch is also
connected to the eduP2P control server with IP address `192.168.0.1`,
and one eduP2P relay server with IP address `192.168.0.2`.

To actually simulate this setup locally on one machine, this test suite
uses Linux network namespaces [\[1\]](#ref-man_network_namespaces). For
example, in order to simulate the network setup above, multiple network
namespaces are configured as in the following diagram:

![](./nat_simulation/network_namespaces.png)

Each network namespace is isolated, meaning that a network interface in
one namespace is not aware of network interfaces in other namespaces. To
allow communication between two namespaces, some network interfaces form
virtual ethernet (veth) device pairs. Since packets on one device in
such a pair also reach the other device in the pair, they can act as
bridges between network namespaces.

There are also network interfaces that only have to communicate with
other interfaces in the same namespace. These interfaces are implemented
using TUN devices, which are virtual network devices that can handle IP
packets.

In this diagram, network namespaces are used to simulate two private
networks, each containing one peer. However, the namespaces are designed
in such a way that extra private networks, or extra peers in each
private network, can easily be added.

Below, an explanation of each namespace and the devices within them is
given. Since the namespaces on the left and right side of the diagram
are very similar, the namespaces on the right side are skipped.

-   private1\_peer1: to allow multiple peers in one private network to
    use eduP2P with userspace WireGuard, each peer needs its own network
    namespace. This is because eduP2P creates a TUN device called `ts0`
    with userspace WireGuard, and only one such device can exist per
    namespace.

    To make sure peers within a private network can still reach their
    router and each other, each peer has a veth pair. Both devices in
    the pair have the same name as the peer’s namespace, with one device
    residing in this namespace, while the other resides in the private
    network’s namespace.

-   private1: each private network needs its own namespace to properly
    isolate the private networks from the public network.

-   router1: a separate network namespace is necessary for each router
    in order for NAT to be applied in the router. This test suite uses
    nft [\[2\]](#ref-man_nft) to apply NAT, and in this framework NAT is
    only applied to the source IP of packets if these packets are
    leaving the local machine. The network interface `router1_pub` that
    applies NAT is in its own namespace, so that both packets going to
    the private network and to the public network look as if they are
    leaving the local machine, and hence have NAT applied to them.

    To allow the router to communicate with the public network,
    `router1_pub` forms a veth pair with the `router1` device in the
    public network. Similarly, to allow the router to communicate with
    the private network, there is a veth pair consisting of the devices
    `router1_priv` in the router’s namespace, and `router1` in the
    private network’s namespace.

-   public: this network namespace exists to isolate the whole network
    setup from the machine’s root network namespace, such that the only
    traffic flowing through the namespaces is traffic concerning eduP2P.
    Besides a veth device for each router, this namespace also contains
    a TUN device that acts as a network switch between the routers, and
    TUN devices to simulate the control and relay server of eduP2P.

## Integration Tests

In these tests, the smaller components of the eduP2P client are tested,
such as the lower layers described in [the document describing eduP2P’s
architecture](../ARCHITECTURE.md):

-   The Session
-   Some of the separate Stages:
    -   TODO decide which, if not all

Furthermore, the control server and relay server are also tested.

The tests can be executed manually by running `go test ./test_suite/...`
from the repository’s root directory.

## Performance Tests

TODO

## Test Results

TODO

## Network Address Translation <a name="nat"></a>

### Definition

Network Address Translation, abbreviated as NAT, is a method currently
used in many networks to avoid exhausting the IPv4 address space.
Instead of all devices being directly connected to the internet with a
globally routable IP address, only the NAT device has a globally
routable IP address. The hosts behind the NAT, called the private
network, forward their traffic to this NAT device, which maps the hosts’
private IP addresses and ports which are only routable within this
private network to public IP addresses and ports which are globally
routable.

There are multiple ways to categorize the different variations of NAT.
This test suite assumes the categorization used in RFC 3489
[\[3\]](#ref-rfc3489), because this RFC is about the STUN protocol,
which is employed in eduP2P and can be used to discover the type of NAT
present between a host and the public network.

There is no official standard for NAT devices to follow, but RFC 4787
outlines behavioural properties observed in NATs and recommends some
practices which NATs should follow.

One behavioural property that differs between NATs is whether they use
an Endpoint-Independent Mapping (EIM). With such a mapping, a private
address-port pair is always translated to the same public address-port
pair, regardless of the desination address and port (called the
endpoint).

### Relevance to eduP2P

Establishing peer-to-peer (P2P) connections between two hosts becomes
more complicated if there is one or multiple NATs on the route between
the hosts. Peers may not know how to reach eachother because they do not
have a globally routable IP address when behind a NAT. Furthermore, even
if translated address of a host behind a NAT is known, packets sent to
this host could still be dropped by some NATs.

To solve these problems, eduP2P uses a combination of a STUN server
[\[4\]](#ref-rfc8489) and UDP hole punching techniques
[\[5\]](#ref-ford2006). With a globally routable STUN server, two hosts
can discover each other’s translated addresses. Then, they can “punch a
hole” in their own NATs by sending packets to each other, such that
their NATs will accept each other’s incoming packets and a direct
connection can be established.

This NAT traversal technique may not work reliably, or at all, depending
on the presence and behaviour of NATs between two hosts that attempt to
establish a P2P connection. In this test suite, the functionality of
eduP2P is verified in various scenarios involving NATs. Furthermore, its
performance will also be measured in terms of bandwidth, latency et
cetera.

Note that this NAT traversal technique does not work if both hosts are
behind a NAT that does not use an EIM. In this case, the STUN server is
unable to discover the translated address used by the hosts when
connecting to each other, since it will differ from the address used
when the hosts connect to the STUN server. Therefore, neither hosts can
discover each other’s translated address to make a connection using UDP
hole punching techniques.

## Bibliography

<span class="csl-left-margin">\[1\]
</span><span class="csl-right-inline">“<span class="nocase">network\_namespaces -
overview of Linux network namespaces</span>.” in
<span class="nocase">Linux Programmer’s Manual</span>. Michael Kerrisk.
Available:
<https://man7.org/linux/man-pages/man7/network_namespaces.7.html></span>

<span class="csl-left-margin">\[2\]
</span><span class="csl-right-inline">P. McHardy and P. N. Ayuso,
“<span class="nocase">nft - Administration tool of the nftables
framework for packet filtering and classification</span>.” Available:
<https://www.netfilter.org/projects/nftables/manpage.html></span>

<span class="csl-left-margin">\[3\]
</span><span class="csl-right-inline">J. Rosenberg, C. Huitema, R. Mahy,
and J. Weinberger, “<span class="nocase">STUN - Simple Traversal of User
Datagram Protocol (UDP) Through Network Address Translators
(NATs)</span>.” in Request for comments. RFC 3489; RFC Editor, Mar.
2003. doi: [10.17487/RFC3489](https://doi.org/10.17487/RFC3489).</span>

<span class="csl-left-margin">\[4\]
</span><span class="csl-right-inline">M. Petit-Huguenin, G. Salgueiro,
J. Rosenberg, D. Wing, R. Mahy, and P. Matthews,
“<span class="nocase">Session Traversal Utilities for NAT
(STUN)</span>.” in Request for comments. RFC 8489; RFC Editor, Feb.
2020. doi: [10.17487/RFC8489](https://doi.org/10.17487/RFC8489).</span>

<span class="csl-left-margin">\[5\]
</span><span class="csl-right-inline">B. Ford, P. Srisuresh, and D.
Kegel, “Peer-to-peer communication across network address translators.”
2006. Available: <https://arxiv.org/abs/cs/0603074></span>
