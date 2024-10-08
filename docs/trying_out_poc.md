# Trying out the Proof of Concept

1. Clone the repository
   - `git clone https://github.com/edup2p/common`
2. Make sure you have (at least) golang 1.22 installed

    Next, if you have not been given credentials to join a control server, first set up the servers as detailed in [this documentation](./prototype_cookbook.md#setting-up-the-servers).
    
    Build the servers with `go build`
    
    Afterward, you should have a:
    - Control Key
    - Control IP
    - Control Port

3. Then, under `cmd/toverstok/`, run `go build` to produce a runnable binary.

4. Next, it depends on whether you wish to run wireguard externally, or in the same process as edup2p itself.

    The instructions on how to set up external wireguard are detailed [here](../cmd/toverstok/README.md#wgctrl-configuration).
    
    The instructions on how to set up in-process wireguard are detailed [here](../cmd/toverstok/README.md#userspace-wireguard-configuration).
    
    The simplest option is to use in-process ("userspace") wireguard, since it will setup everything by itself.
    
    On windows, userspace wireguard is the only option (for now).
    
    If using userspace wireguard, privilege escalation is required, see [the same segment](../cmd/toverstok/README.md#userspace-wireguard-configuration) for more information.
    
    On MacOS and Linux, run toverstok with `sudo ./toverstok`.

    On Windows, run `toverstok.exe` in an elevated prompt.

5. Input the following lines, to set up and connect. Replace `CONTROL_*` with the key, IP, and port gotten above.

    ```
    log debug
    key file
    wg usr
    
    pc key CONTROL_KEY
    pc ip CONTROL_IP
    pc port CONTROL_PORT
    pc use
    
    en create
    en start
    ```

6. Use `ip addr` (windows: `ipconfig`) to observe your virtual IP address.