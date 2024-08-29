# eduP2P

An authenticated peer-to-peer network overlay.

Development notes can be found in [`DEVELOPING.md`](./DEVELOPING.md).

Library usage can be found in [`toversok/`](./toversok/README.md).

A proof-of-concept minimal implementation (wrapping the library) can be found in [`cmd/toverstok/`](./cmd/toverstok/README.md).

**eduP2P is currently in the proof-of-concept stage**, no fully fledged clients exists yet, but there is a working
terminal-based development program in `cmd/toverstok/`, as mentioned above.

Documentation on how to use the PoC (from scratch) can be found [here](./docs/trying_out_poc.md)

## Design

Toversok - the internal common-component library - provides the core of the overlay network that makes peer-to-peer connections possible.

On a high level;
- It connects to a control server, to receive peer definitions, keys, assigned IP addresses of those peers, and information about relay servers.
- On-demand, it connects to one relay server permanently, and will connect to other relay servers on-demand when sending data to their attached peers.
- It will opportunistically try to establish direct connection to peers, using relays as a fallback.

![](./docs/high_level.png)

For an in-depth overview of the design, you can look at [ARCHITECTURE.md](./ARCHITECTURE.md).