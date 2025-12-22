# The client-to-control protocol

## High Level

After establishing a HTTP(S) connection to a Control Server's HTTP(S) endpoint, the HTTP connection is upgraded/switched
to the control protocol.

This protocol is a framed protocol, with a header of;

- `type`, a single byte
- `length`, 4 bytes of a `uint32`, big-endian

Following the header, is a `length`-sized blob, that contains ASCII JSON.

## Message Types

Message type values are documented in `types/msgcontrol/msg.go`.

The following headers are not in the same order as in the above file, to make reading and understanding the various
types
easier.

### Handshake Messages

The following message types are supposed to only be used during the handshake phase.

#### `ClientHello`

Sent by the client directly after establishing a connection with the control server, sending the client's public key.

#### `ServerHello`

Sent by the server directly after `ClientHello`, sending the server's public key, and check data, encrypted with the
control server's private key, and the client's public key. This data is used in `Logon`.

#### `Logon`

Sent by the client directly after `ServerHello`, this includes;

- The client's generated session key.
- Re-encrypted check data with the client's key.
- Re-encrypted check data with the session key.
- An optional session ID, to ask for a resumption.

#### `LogonAuthenticate`

Can be optionally sent by the control server after `Logon`; Indicates to the client that they need to show a user
prompt with the given URL, to continue authentication through external means.

Contains a string of the authentication URL.

#### `LogonDeviceKey`

Can be optionally sent by the client after `Logon`; Indicates to the control server that the client has a special
"device key" that it wants to use to authenticate itself with the server. It is up to the control server's business
logic to accept or reject this.

Contains an opaque string with the device key.

#### `LogonAccept`

Sent after `Logon`, `LogonAuthenticate`, or `LogonDeviceKey` to indicate the end of the handshake phase, and to
continue the client to the session phase.

It contains;

- The client's virtual IPv4 and IPv6 addresses.
- The expiry time of the client's current authentication, set by business logic.
- The current session ID.

#### `LogonReject`

Sent after `Logon`, `LogonAuthenticate`, or `LogonDeviceKey` to indicate the end of the handshake phase, and to
close the connection with a rejection.

It contains;

- An opaque string describing the rejection reason.
- An optional retry strategy code, telling the client to retry without a resumption session ID, or with a new Session
  Key, or if retrying is not possible (which is also the default, if omitted).
- An optional "retry after" string-encoded golang time duration, which indicates how long the client should wait
  until retrying again.

## Session messages

#### `EndpointUpdate`

Sent by the client, to indicate an update in its advertised endpoints.

Contains the aforementioned endpoints, in an array.

#### `HomeRelayUpdate`

Sent by the client, to indicate an update in its advertised "home" relay.

Contains the ID of the aforementioned relay.

#### `PeerAddition`

Sent by the control server, for the client to become aware of an additional client (peer).

Contains initial peer information, such as;

- Public Key
- Session Key
- Virtual IPv4 and IPv6 addresses
- Endpoints (array)
- Home Relay
- Additional properties (Quarantine, MDNS)

Also sent after `LogonAccept`, to make a client aware of all their peers.

#### `PeerUpdate`

Sent by the control server, to update the client's information on other clients (peers).

Contains updated peer information, in optional fields, such as;

- Public key (identifying)
- New session key, if present
- New endpoints, if present
- New home relay, if present
- New additional properties, if present

#### `PeerRemove`

Sent by the control server, to inform the client that another client is offline, or shouldn't be regarded, anymore.

Contains the identifying public key.

#### `RelayUpdate`

Sent by the control server, to inform the client of relay updates.

Contains an array of relays. All relays are identified by their ID, and only updates existing relay information,
never replacing the set of known relays of a client.

Also sent immediately after `LogonAccept`.

#### `Logout`

Sent by the client to the control server, to request closing the connection gracefully, and invalidating the current
session.

#### `Disconnect`

Sent by the control server, to inform the client that the connection is closing.

This contains, just as `LogonReject`;

- An opaque string describing the disconnection reason.
- An optional retry strategy code, telling the client to retry without a resumption session ID, or with a new Session
  Key, or if retrying is not possible (which is also the default, if omitted).
- An optional "retry after" string-encoded golang time duration, which indicates how long the client should wait
  until retrying again.

#### `Ping`

Sent by the control server, expects a `Pong` reply.

Contains check data for the client to re-encrypt, and return.

#### `Pong`

Sent by the client, after a `Ping` message.

Contains re-encrypted check data, using the client's key, and session key.

## Handshake Phase

The handshake phase of the connection are detailed in [`handshake.mermaid`](../../types/control/handshake.mermaid).

TODO