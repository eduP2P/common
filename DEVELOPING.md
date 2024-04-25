# Development Guide

## Architecture

The architecture of this repository is roughly as thus;
- `toversok/` contains all the *library components* for the P2P engine, to be embedded in any Client.
- `server/` contains library components for all the *server components*
  - `server/control/` contains library functionality for the Control and Coordination server (named "control" for ease),
    establishing and distributing session keys and endpoints.
  - `server/relay/` contains library functionality for the Relay server, which will provide nodekey-based relaying services.
  - `server/stun/` provides library functionality for the STUN server.
- `types/` contains common types between toversok, servers, and clients.
- `cmd/` contains ready-to-run applications, both server, development, and client-based.