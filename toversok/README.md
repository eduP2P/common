# ToverSok
> *(Dutch) "Magic Sock"*

A library comitted to simplifying the following functionality;
- Proxying wireguard connections
- Establishing peer-to-peer connections, directly or via relay
- Transferring packets to remote nodes
- Gracefully failing over to relays once direct connections fail

## Library Use

The main library object is `Engine`, which holds all other objects, and maintains a session.

The interfaces to implement are `WireGuardHost`, `FirewallHost`, and `ControlHost`.

`ControlHost` is an interface that creates a client, given a private key, and a session key.

`WireGuardHost` and `FirewallHost` generate "Controllers". There's a 1:1 correspondence between controllers and sessions.

`WireGuardController` controls the internal or external wireguard implementation,
and provides per-peer connections with which to receive and send wireguard frames.

Internally, objects are structured like this;

- Engine
  - Session
    - Stage

Engine monitors sessions, and replaces them when one session fails.

Sessions hold controllers and the stage, and facilitate communication between them.

Stages hold an internal actor-message model, sending frames and updates to eachother,
for high throughput, and parallelism.

## Key structure

In total, there are 3 kinds of keys, each have their public and private types.

(All of these can be found in [`../types/key`](../types/key))

- Node Key
  - Used as a wireguard key
  - Used to authenticate node to control server
- Session key
  - Generated on every new session
  - Used to encrypt peer-to-peer session messages (rendezvous, ping, etc.)
- Control Key
  - Used to mutually verify with the client, and encrypt communications