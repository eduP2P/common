// Package peer_state contains a state machine that tries to drive a direct connection with another peer to establishment.
//
// In short, it tries to NAT-punch opportunistically, backing off, handling glare, and only establishing a connection
// after a full loop has been established. The same happens on the other peer, and direct connections are retried once
// pongs haven't been received for 5 seconds (with pings at 2 second intervals).
//
// See [peer_state.mermaid] for a primary reference of this state machine.
package peer_state
