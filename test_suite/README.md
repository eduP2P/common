# Test Suite

This repository uses Continuous Integration (CI) to automatically run
the tests when the repository is updated. CI is implemented using GitHub
Workflows, and the workflow running the tests can be found
[here](.github/workflows/go.yml).

The test suite contains three types of tests:

1.  System tests to verify the functionality of the whole system.
2.  Integration tests to verify the functionality of smaller parts of
    the system.
3.  Performance tests to measure metrics such as the delay, jitter and
    throughput of the peer-to-peer connection.

They are described in more detail below.

## System Tests

In these tests, two clients attempt to establish a peer-to-peer
connection using eduP2P. The test suite aims to test each possible
combination of the following variables:

-   The presence of one or more NATs in front of a client. These NATs
    may have varying behaviour, as described below:
    -   TODO
-   Whether a client uses IPv4 or IPv6.

The tests can be executed manually with [this script](setup.sh).

In these tests, Network Address Translation (NAT) devices have to be
simulated. An overview and how it is used in the test suite is given in
a later section of this document.

## Integration Tests

In these tests, the smaller components of the eduP2P client are tested,
such as the lower layers described in [the document describing eduP2P’s
architecture](../ARCHITECTURE.md): - The Session - Some of the separate
Stages: - TODO decide which, if not all

Furthermore, the control server and relay server are also tested.

The tests can be executed manually by running `go test ./test_suite/...`
from the repository’s root directory.

## Performance Tests

TODO

## Results

TODO

# Network Address Translation

## Definition

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

## Relevance to eduP2P

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
NAT that does not use an EIM. In this case, the STUN server is unable to
discover the translated address used by the hosts when connecting to
each other, since it will differ from the address used when the hosts
connect to the STUN server. Therefore, neither hosts can discover each
other’s translated address to make a connection using UDP hole punching
techniques.

# Bibliography

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
