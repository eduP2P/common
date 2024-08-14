# Wireguard Userspace

WIP: This folder contains a `WireGuardHost` ran in userspace. 

See this ongoing issue for more context: https://github.com/ShadowJonathan/eduP2P/issues/49.

On all platforms; The userspace wireguard implementation needs sufficient permissions to;
- Create a network interface
- (MacOS/Linux) Run `ip`/`route` commands.

For now, practically, this requires `sudo` on linux and macos, and `gsudo` on windows.

(See this issue for more information about permission refinement: https://github.com/ShadowJonathan/eduP2P/issues/56)

This implementation will configure its own routing, interface, and IP address, as given by Control.

As for now, `NewUsrWGHost()` will create a new userspace host, which can be passed to `toversok.Engine` directly.

Permission errors bubble up at `(*toversok.Engine).Start()`.