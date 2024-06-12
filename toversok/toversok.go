// Package toversok contains the main types of its peer-to-peer running engine.
//
// In rough terms, the Engine is meant to be a programmer's primary interface for implementation,
// it has to be fed a ControlHost, WireGuardHost, and a FirewallHost, to then do its job properly.
//
// The Engine will create a Session, which in turn will create an actors.Stage. The Session is able to be
// deconstructed and recreated on demand, and the Engine will do automatically after first logon.
package toversok
