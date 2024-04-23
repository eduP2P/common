TMan (Traffic Manager) contains logic that will try to holepunch NATs periodically if it detects that a connection is being used.

The Mermaid State Diagram outlines this internal process.

```mermaid
stateDiagram-v2
    wfs: Waiting For Info

    I: Inactive

    R: Trying

    Eing: Establishing

    E: Established

    [*] --> wfs

    wfs --> I

    I --> R: Connection Active,\n(re)try immediately

    I --> Eing: Received Rendezvous
    R --> Eing: Retry Timer Fires

    R --> Eing: Received Rendezvous
    
    I --> I: Ping>Pong
    R --> R: Ping>Pong

    state Eing {
        T: Transmitting
        PT: Pre-Transmit (T)
        hE: Half-Established
        GR: Rendezvous Got (T)
        RR: Rendezvous Acknowledged
        he_pre: Half-Establishing (T)
        F: Finalizing (T)

        %% note left of T: Send Pings and Rendezvous
        %% note left of hE: Send Pong + Ping
        %% note right of RR: Send Pings

        %%        state rr_join <<join>>

        [*] --> PT
        PT --> T: Send Pings and Rendezvous\n(transient)
        [*] --> GR: Got Rendezvous
        T --> GR: Got Rendezvous
        GR --> RR: Send Pings\n(to Rendezvous endpoints)\n(transient)

        %% T --> hE: Got Ping

        RR --> he_pre: Got Ping
        T --> he_pre: Got Ping
        
        he_pre --> hE: Send Pong + Ping\n(transient)
        
        hE --> hE: Ping>Pong

        RR --> F: Got Pong
        hE --> F: Got Pong
        T --> F: Got Pong
        
        F --> [*]

    }
    
    B: Booting (T)
    
    TD: Teardown (T)
    
    Eing-->B
    
    B --> E: Send addrpair to outconn,\nregister addrpair aka to dman

%%    Eing --> E: Got Pong

    Eing --> R: Timeout of 10s,\nretry after 40s

    E --> TD: Either ping or pong not received in last 5s,\nretry immidiately
    E --> TD: Connection Inactive
    
    state td_join <<join>>
    
    TD --> td_join: clead dman aka\nsend home relay to outconn
    
    td_join --> I
    td_join --> R

    E --> E: Send pings every 2s
    E --> E: Ping>Pong
```

The follow diagram outlines the interactions between different actors:

```mermaid
flowchart TB
    WG[WireGuard]
    L((Local Socket\nRecvLoop))
    WGR{{Wireguard Writer}}

    subgraph Connections
        POC{Peer\nOutConn}
        PIC{Peer\nInConn}
    end

    TM{Traffic\nManager}
    SM{Session\nManager}
    
    subgraph Direct
        DR{Direct\nRouter}
        DM{Direct\nManager}
        DM <==> DS[Lots of sockets\nSending and Receiving]
    end

    subgraph Relay
        RR{Relay\nRouter}
        RM{Relay\nManager}
        RM <==> RC[Relay Connections]
    end

    POC ==> |Write to\nAddrPort for DST| DM
    POC ==> |Write to\nRelay for DST| RM
    DM ==> |Received Packet\nFrom AddrPort| DR
    RM ==> |Received Packet\nFrom Relay| RR
    DR & RR ==> |Forward WireGuard Frames| PIC

    WG ==> |1:M| L
    L ==> |1:1| POC
    PIC ==> WGR
    WGR ==> WG
    
    SM --> |Session Messages\nFrom Relay/AddrPort| TM
    TM --> |Session Messages\nTo Relay/AddrPort| SM
    SM --> |Session Frames\nTo AddrPort| DM
    SM --> |Session Frames\nTo Relay| RM
    DR --> |Session Frames from AddrPort| SM
    RR --> |Session Frames from Relay| SM

    POC -.-> |ConnOut\nActive/Inactive| TM
    PIC -.-> |ConnIn\nActive/Inactive| TM
    TM -.-> |Use Relay/AddrPort| POC
    TM -.-> |AddrPort X\nAKA Peer Y| DR
```