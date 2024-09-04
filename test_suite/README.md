# Network Address Translation

Network Address Translation, abbreviated as NAT, is a method created to
avoid the problem of the IPv4 address space being too small for the
amount of devices connected to the internet. Instead of all devices
being directly connected to the internet with a globally routable IP
address, only the NAT device has a globally routable IP address. All
traffic goes via this NAT device, which maps the other devices’ internal
IP addresses and ports which are only routable within the local network
to external IP addresses and ports which are globally routable.

There are multiple ways to categorize the different variations of NAT.
This test suite assumes the categorization used in RFC 3489
[\[1\]](#ref-rfc3489), because this RFC is about the STUN protocol,
which is employed in eduP2P and can be used to discover the type of NAT
present between a host and the public network.

RFC 3489 categorizes NATs into the following four variations:

-   **Full Cone**: in this variation, each combination of internal IP
    address and port is always mapped to the same external IP address
    and port. Therefore, packets sent by an external host to a mapped
    external address will be forwarded to the corresponding internal
    host.
-   **Restricted Cone**: this variation is similar to the previous, but
    packets sent by an external host to a mapped external address will
    only be forwarded to the corresponding internal host if this
    internal host previously sent a packet to the external host’s IP.
-   **Port Restricted Cone**: this variation is like the restricted cone
    NAT, but for packets to be forwarded to the internal host, a
    previous packet must have been sent to the external host’s IP *and*
    port.
-   **Symmetric**: in this variation, the mapping depends on the
    internal address *and* the destination address: each request from an
    internal IP address and port to a destination IP address and port is
    mapped to the same external IP address and port.

Establishing peer-to-peer (P2P) connections between two hosts becomes
more complicated if there is one or multiple NATs on the route between
the hosts. Peers may not know how to reach eachother because they do not
have a globally routable IP address when behind a NAT. Furthermore, even
if translated address of a host behind a NAT is known, packets sent to
this host in the case of a (port) restricted cone NAT could still be
dropped.

To solve these problems, eduP2P uses a combination of a STUN server and
UDP hole punching techniques [\[2\]](#ref-ford2006). With a globally
routable STUN server, two hosts can discover each other’s translated
addresses. Then, they can “punch a hole” in their own NATs by sending
packets to each other, such that their NATs will accept each other’s
incoming packets and a direct connection can be established.

Note that this NAT traversal technique does not work if both hosts are
behind a symmetric NAT. In this case, the STUN server is unable to
discover the translated address used by the hosts when connecting to
each other, since it will differ from the address used when the hosts
connect to the STUN server. Therefore, neither hosts can discover each
other’s translated address to make a connection using UDP hole punching
techniques.

In the case of symmetric NATs, the hosts can communicate via a globally
routable relay server using the TURN protocol [\[3\]](#ref-rfc5766).

# Bibliography

<span class="csl-left-margin">\[1\]
</span><span class="csl-right-inline">J. Rosenberg, C. Huitema, R. Mahy,
and J. Weinberger, “<span class="nocase">STUN - Simple Traversal of User
Datagram Protocol (UDP) Through Network Address Translators
(NATs)</span>.” in Request for comments. RFC 3489; RFC Editor, Mar.
2003. doi: [10.17487/RFC3489](https://doi.org/10.17487/RFC3489).</span>

<span class="csl-left-margin">\[2\]
</span><span class="csl-right-inline">B. Ford, P. Srisuresh, and D.
Kegel, “Peer-to-peer communication across network address translators.”
2006. Available: <https://arxiv.org/abs/cs/0603074></span>

<span class="csl-left-margin">\[3\]
</span><span class="csl-right-inline">P. Matthews, J. Rosenberg, and R.
Mahy, “<span class="nocase">Traversal Using Relays around NAT (TURN):
Relay Extensions to Session Traversal Utilities for NAT (STUN)</span>.”
in Request for comments. RFC 5766; RFC Editor, Apr. 2010. doi:
[10.17487/RFC5766](https://doi.org/10.17487/RFC5766).</span>
