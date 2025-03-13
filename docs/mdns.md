# mDNS Notes

[mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) (multicast DNS) is defined in
[RFC 6763](https://datatracker.ietf.org/doc/html/rfc6762) as, essentially, UDP DNS packets sent to broadcast addresses
`224.0.0.251` and `FF02::FB` on port `5353`.

On top of that, a new `UNICAST-RESPONSE` (`"QU"`) bit is added to the query section, which can be parsed as `QCLASS`
`2^15`, and a `CACHE-FLUSH` bit on every resource (answer/additional/authority) record, which can be parsed as `RRCLASS`
`2^15`.

Exact implementation differs per operating system, but as a rule of thumb;

- mDNS (like most broadcast packets) aren't sent over `PPP` (point to point) classes of networks, which Wireguard is.
- Linux needs an additional system component to enable mDNS, such as "Avahi".
- MacOS, Linux, and Windows have differently covering implementations;
  f.e. Windows doesn't allow unicast queries, while macOS does.
- While mDNS works via loopback, some operating systems have quirks with how they work, and only fully work mDNS via "
  regular" LAN interfaces.

## Intercepting mDNS

Because of the above limitations (no mDNS on PPP, etc.), we cannot intercept mDNS packets via TUN (level 3, IP),
and would have to listen to them on the regular interfaces, by listening on port `5353` (and fight with the system
listener to have it share its port),
grab mDNS packets, filter them (to prevent noise from the local LAN), transform them (to point to the right IP address
over the Wireguard interface), send them over to interested parties, and then inject them.

This essentially makes mDNS packets get wiretapped, and "appear out of thin air" at the recipient,
which should tie it together.