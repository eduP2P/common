# ToverStok
> *(Dutch) "Magic Wand"*

A small application meant to drive the toversok library via shell interface.

This application can provide a way to prototype/develop/test toversok/eduP2P clients.

(The current implementation is hardcoded to use wgctrl/wg-tools to communicate with an external wireguard implementation)

## Command Reference

### Log Commands

```
log
    Get log level.

log info
    Sets to "info" level of logging.

log debug
    Sets to "debug" level of logging. A bit more verbose than info.

log trace
    Sets to "trace" level of logging. This includes direct connection states, and received session/control messages.
```

### Node Key

These commands pertain to setting, getting, and generating the private key variable in the shell environment.

This key will be used when creating a new Engine.

```
key
    Get the current private key variable.

key gen
    Generate a new private key (and set it).

key set ["privkey:HEX"]
    Set the private key from command line arguments, or alternatively line-input.
    
    Must be a hex string prefixed with 'privkey:', optionally wrapped in double quotes.

key pub
    Print the public key from the currently set private key.
```

### Wireguard Commands

These commands pertain to setting up a wireguard host/implementation in the shell environment,
used when creating a new Engine.

```
wg
    Get the current wireguard state.

wg use [device_iface]
    Initialises a wireguard host with the specified device.
    
    If none are specified, will enter a multi-select mode with the currently detected availible devices.

wg init <privkey_hex> <ipv4/cidr> <ipv6/cidr>
    Perform MANUAL INITIALISATION on the wireguard host.
    
    This calls Init() on the wireguard interface, supplying it the private key (parsed from hexstring), and virtual IPs.
    
    This step is NOT neccecary when supplying the wireguard interface to an engine.
```

### Proper Control Commands

These commands pertain to setting up variables for connecting to a "proper" host server.

This is in contrast with the "fake" control commands, which set up an internal relay,
to address the client directly with peer and relay definitions when it is initialised.

```
pc
    Print current "proper control" variables, such as dial opts, and control key.

pc use
    Tell the shell environment to use proper control when creating engines.

pc key ["control:HEX"]
    Set the expected control public key from command line arguments, or alternatively line-input.
    
    Must be a hex string prefixed with 'control:', optionally wrapped in double quotes.

pc domain [domain name]
    Set the domain that control is hosted on. Will read from line-input if no argument is set.
    
    Will resolve IP addresses from this domain name if no ip addresses are set.

pc ip [ip address]
    Set the ip address that control is hosted on. Will read from line-input if no argument is set.
    
    Will override domain name resolution if set.
    
pc port <port number>
    Set the port that control is hosted on.
```

### Fake Control Commands

These commands pertain to handling "fake" control, feeding the client definitions via command line input.

```
fc use
    Tell the shell environment to use fake control when creating engines.

fc peer add(/a) <"pubkey:HEX"> <relay ID> <ip4> <ip6> <addrport endpoints...>
    Add a peer to the client, defining its pubkey, home relay, virtual ipv4 and ipv6 addresses,
    and endpoints that it can be directly contacted on.
    
    Note that this command should not be used again when making changes to peers,
    delete the peer with 'fc peer delete' first.

fc peer delete(/del/d) <"pubkey:HEX">
    Delete a peer from the client, by its pubkey.

fc relay <relay ID> <"pubkey:HEX"> [FLAGS]
    Define or update a relay, according to its ID.
    
    FLAGS:
        -d [domain]
            Set the domain that this relay can be resolved from.
        
        -a [ip,...]
            Set the IP address(es) that this relay can be contacted at.
            Overrides domain resolution.
        
        -s [stunPort]
            Set the STUN port that this relay uses, defaults to 3478.
        
        -t [httpsPort]
            Set the HTTPS port that this relay uses, defaults to 443.
        
        -h [httpPort]
            Set the HTTP port that this relay uses, defaults to 80.
        
        -i
            Flag, for whether to use 'insecure' means to connect to the relay.
            (This will use the HTTP port for the relay connection, instead of HTTPS)

fc ip4 [ip4/cidr]
    Get or set the current IPv4 address + expected network that will be sent to the client.
    
    This uses CIDR notation, and will not ignore trailing bits.
        (So 10.42.69.10/24 will have the client use 10.42.69.10 as its virtual IP address,
        and define 10.42.69.0/24 as the expected network it can contact other clients on.)

fc ip6 [ip6/cidr]
    Get or set the current IPv6 address + expected network that will be sent to the client.
    
    This uses CIDR notation, and will not ignore trailing bits.
```

### Engine Commands

These commands pertain to setting up and starting the engine.

When creating the engine, the shell environment will insert variables that can be set with the above commands.

```
en
    Get engine created/started status.

en port
    Get/sets a manual external port that the engine will bind a UDP socket to that will be used for direct connections.
    
    Default to 0; let the system allocate a random port.

en create
    Creates a new engine from shell environment variables. Performs checks for unset variables.

en start
    Starts the engine.
    
    It will attempt immidiately to connect to the control server. If this fails, it will return an error,
    and the engine will not be started.
    
    If control connection issues arise after starting, it'll will restart automatically.
```

## Example Flow

Here is an example set of commands to run when connecting to a proper control server.

Replace `PRIVHEX`, `WGDEV`, `CTRLHEX`, `CTRLIP`, `CTRLPORT` with the appropriate values.

```
log trace

key set privkey:PRIVHEX

wg use WGDEV

pc key control:CTRLHEX
pc ip CTRLIP
pc port CTRLPORT
pc use

en create
en start
```

## wgctrl configuration

For toverstok, wgctrl is used, which needs an externally running wireguard implementation.

On MacOS, the wireguard-go binary can be used.

On Linux, kernel wireguard can also be used.

**You will get a message on engine creation to run a few commands, these are required to get networking to run properly.**

### MacOS - Userspace

> *Please be aware this userspace wireguard implementation has lot throughput rates (20mbps)*

Create a utunXX interface with the following commands:
1. Install wireguard-go: `brew install wireguard-go`
2. Run an instance as sudo: `sudo wireguard-go utun`
3. Add ownership of current user to wireguard sockets: `sudo chown $USER /var/run/wireguard/utun*`

To shut down the sockets, run `sudo rm /var/run/wireguard/utun*`

### Linux - Kernel

On linux, add the `NET_ADMIN` capability to the (compiled) toverstok binary with
`setcap cap_net_admin=ep toverstok`, or alternatively run as root.

Create the wg0 interface with the following commands:
1. Create the wg0 interface: `sudo ip link add wg0 type wireguard`
2. Set up the interface: `sudo ip link set wg0 up`