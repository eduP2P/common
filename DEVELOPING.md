# Development Guide

## Architecture

The architecture of this repository is roughly as thus;
- `toversok/` contains all the *library components* for the P2P engine, to be embedded in any Client.
- `server/` contains library components for all the *server components*
  - `server/control/` contains an implementation for the Control and Coordination server (named "control" for ease),
    establishing and distributing session keys and endpoints.
  - `server/relay/` contains an implementation for the Relay server, which will provide nodekey-based relaying services.
- `types/` contains common library types.
- `cmd/` contains various miscellaneous (small) applications, mainly for development or maintenance purposes.
