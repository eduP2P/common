# eduP2P Test Suite

## Table of Contents

1.  [Overview](#overview)
2.  [Requirements](#requirements)
3.  [Network Address Translation](#network-address-translation)
4.  [Bibliography](#bibliography)

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

-   Docker Engine, which can be installed
    [here](https://docs.docker.com/engine/install/ubuntu/).
-   Go version 1.22+, which can be installed
    [here](https://go.dev/doc/install) and is necessary to build eduP2P.

### System test-specific requirements

To run the system tests, two new network interfaces have to be created
to simulate networks between the Docker host and the two peers which run
in Docker containers. The following commands create two Docker networks
with IPv6 enabled:

    docker network create --ipv6 --subnet fd42:7e57:c0de:1::/64 --opt com.docker.network.bridge.name=peer1 peer1

    docker network create --ipv6 --subnet fd42:7e57:c0de:2::/64 --opt com.docker.network.bridge.name=peer2 peer2

If the networks are configured correctly: - In the outputs of
`ip addr show peer1` and `ip addr show peer2`, you will see one of the
subnets specified above. - In the outputs of
`docker network inspect peer1` and `docker network inspect peer2`, the
key “EnableIPv6” is set to true.

Finally, the `DOCKER-USER` chain of iptables needs to be configured to
forward packets between the two networks:

    sudo iptables -I DOCKER-USER -i peer1 -o peer2 -j ACCEPT
    sudo iptables -I DOCKER-USER -i peer2 -o peer1 -j ACCEPT

## System Tests

In these tests, two clients attempt to establish a peer-to-peer
connection using eduP2P. When these tests are executed via GitHub
workflows, the test results can be found in the output of the ‘test’ job
under the step ‘System tests’, and the logs can be downloaded under the
‘Artifacts’ tab. The system tests can also be executed manually with
[this script](setup.sh).

In these tests, Network Address Translation (NAT) devices have to be
simulated. An overview of NAT and how it is used in the test suite is
given in a later section of this document.

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

## Results

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
[\[1\]](#ref-rfc3489), because this RFC is about the STUN protocol,
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
[\[2\]](#ref-rfc8489) and UDP hole punching techniques
[\[3\]](#ref-ford2006). With a globally routable STUN server, two hosts
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
</span><span class="csl-right-inline">J. Rosenberg, C. Huitema, R. Mahy,
and J. Weinberger, “<span class="nocase">STUN - Simple Traversal of User
Datagram Protocol (UDP) Through Network Address Translators
(NATs)</span>.” in Request for comments. RFC 3489; RFC Editor, Mar.
2003. doi: [10.17487/RFC3489](https://doi.org/10.17487/RFC3489).</span>

<span class="csl-left-margin">\[2\]
</span><span class="csl-right-inline">M. Petit-Huguenin, G. Salgueiro,
J. Rosenberg, D. Wing, R. Mahy, and P. Matthews,
“<span class="nocase">Session Traversal Utilities for NAT
(STUN)</span>.” in Request for comments. RFC 8489; RFC Editor, Feb.
2020. doi: [10.17487/RFC8489](https://doi.org/10.17487/RFC8489).</span>

<span class="csl-left-margin">\[3\]
</span><span class="csl-right-inline">B. Ford, P. Srisuresh, and D.
Kegel, “Peer-to-peer communication across network address translators.”
2006. Available: <https://arxiv.org/abs/cs/0603074></span>
