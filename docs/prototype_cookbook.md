# Prototype Setup Cookbook

This document will describe how to set up a working p2p network using the prototype Control Server (`cmd/control_server/`), Relay Server (`cmd/relay_server/`), and Client (`cmd/toverstok/`).

## General Architecture

The general idea for the P2P network architecture is so;

- Clients register and login to a Control Server.
- The Control server tells them about associated Relay Servers, and other Clients (peers).
- The Clients connect to Relay Servers, and use them to exchange control messages with other Clients.
- The Clients establish direct or relayed connections, for IP-level traffic between them.

## Setting up the servers

When setting up the servers in this prototype architecture, 
it is recommended to put them on public IP addresses.

Though, for the purpose of prototyping, putting all servers in the same "context" (local machine, LAN, Public Internet) is also fine.

However: If these boundaries cross (Control server on public IP, relay on LAN), things might break.

The servers are ultimately designed to be facing the public internet, and so global addressability is assumed. (As "global" as the clients themselves will be)

### Setting up a Control Server

The source files for the prototype control server are under `cmd/control_server/`. Build it into a binary.

Take the built binary, and upload it to the server if the purpose is to install this on a remote server.

> Make sure the binary is built for the appropriate architecture and OS,
> see documentation on `GOOS`, `GOARCH` and `go build` on how to do this.

Then, run the binary a first time, to generate a default config;

`./control_server -c ./control.json -a ":9999"`

`-c` sets the configuration path. If empty and the current user is root, will default to `/var/lib/toversok/control.json`.

`-a` sets the address that the control server will listen to. Will default to `:443`.

Be sure to supply `-a` if something else is already running on `:443` on the local machine,
or if the local user has no root/`cap_net_admin` privileges.

Hit `ctrl-c` to exit the booted control server.

Note the Public Key that has been printed in the console window, copy it somewhere for future use.

The virtual IP address space defaults to `10.42.0.0/16` & `fd42:dead:beef::/64`, edit this in `config.json` if you want to change this.

`Relays` will be touched later, keep it an empty array for now.

### Setting up a Relay Server

The source files for the prototype control server are under `cmd/relay_server/`. Build it into a binary.

Take the built binary, and upload it to the server if the purpose is to install this on a remote server.

> Make sure the binary is built for the appropriate architecture and OS,
> see documentation on `GOOS`, `GOARCH` and `go build` on how to do this.

Run the binary for a first time, similar to the control server, to generate a default config:

`./relay_server -c ./relay.json -a "0.0.0.0:3340" -stun-port 3478`

`-c` sets the configuration path. If empty and the current user is root, will default to `/var/lib/toversok/relay.json`.

`-a` sets the address the relay server will listen to. Address must be *explicitly* given, so `0.0.0.0` for a default.

`-stun-port` sets the port that the STUN component will listen on. Address part will be taken from `-a`.

After running for a first time, stop the server with `ctrl-c`, note down the printed public key.

### Adding the Relay Server information to the Control Server

Open `control.json` from the control server.

Control expects relay information to be supplied somewhat to the following specification:

```json5
{
    // Unique ID integer of this Relay.
    // Due to prototype limitations, there;
    // - must always be a `0` relay.
    // - all relay IDs must be unique.
    "ID": 0,
  
    // The Relay's public key.
    // This can be omitted, but when given, will have all clients verify the relay's public key with this one.
    "Key": "pubkey:67bcfd99ac10fdc02ea4e50bd9a64a6890595e30099fe9152c2ffc73ba309e1e",
  
    // The Relay's domain. Optional.
    // If given, and IPs is not set, will be used to resolve IPs from DNS.
    // If given, and CertCN is not set, will be used to check against the certificate Common Name in a TLS connection.
    // Will be used as the `Host` header hostname part when dialing. (Else it'll default to "relay.ts")
    "Domain": "1.relay.edup2p.org",
  
    // The Relay's reachable IPs.
    // Will override "Domain" in IPs connected to.
    // Not optional if "Domain" is not given.
    "IPs": [
        "145.220.52.22",
        "2001:67c:6ec:520:5054:ff:fea9:8200"
    ],
  
    // The STUN Port that this Relay advertises.
    // Optional, defaults to 3478
    "STUNPort": 3478,
  
    // The HTTPS Port that this Relay advertises.
    // Optional, defaults to 443.
    "HTTPSPort": 443,
    
    // The HTTP Port that this Relay advertises.
    // Will/can be used for captive portal checks.
    // Will be used for relaying if "IsInsecure" is true.
    // Optional, defaults to 80.
    "HTTPPort": 80,
  
    // Set to false for when connections to Relays are secured with TLS.
    // Current prototypes dont support TLS connections directly, set to true for almost all cases.
    "IsInsecure": true
}
```

A minimal setup can look like this:

```json
{
    "ID": 0,
    "Key": "pubkey:67bcfd99ac10fdc02ea4e50bd9a64a6890595e30099fe9152c2ffc73ba309e1e",
    "IPs": [
        "145.220.52.22",
        "2001:67c:6ec:520:5054:ff:fea9:8200"
    ],
    "IsInsecure": true
}
```

For the prototype, all clients will default to connecting to relay 0 and using it as its "home" (default incoming) relay.

Create at least this relay, and define it inside the "Relays" array inside `control.json`.

## Running a Client

For the client prototype, currently there is `cmd/toverstok/`, which provides a shell interface to setting up a client,
and monitoring it, with custom values.

More documentation can be found in [`cmd/toverstok/README.md`](../cmd/toverstok/README.md).

Use that documentation to setup platform-specific wireguard, or receive platform-specific wireguard instructions.

A few notes/helps/reminders:

Use `key file` for ease and consistency; the private key of the client will be saved in a file called `toverstok.key`, in the local directory, by default. (Change this file location by supplying another argument with the path)

Alternatively, run `key gen` to generate a private key for each client. Write these down, and keep track of which is which. Then, upon every boot of a client, run `key set` to set the private key of that client again.

Use the `pc` commands to point the client to the control server:

```
pc key control:PUBLIC_KEY
pc ip IP
pc port PORT
pc use
```

Replace `PUBLIC_KEY`, `IP`, and `PORT` with the respective values; public key, accessible IP, and Port.

Finally, when everything is ready on every client, run `en create`, and `en start`.
Any errors will tell you if values are missing or something is wrong.

After `en start`, logs should flood the console.
Run `info` to reduce some of these, and keep the most important ones. (Other levels are `debug` and `trace`)

The most important console message at this point are ones from `wgctrl` pointing out which commands to run on the host system to setup proper networking.
**These commands need to have ran before networking is possible.**

Inside of these commands would be the IPv4 and IPv6 of the local machine. After running this on multiple machines, these machines should now be accessible over IP. Run `ping` for a test.