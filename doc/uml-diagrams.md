# TermChat UML Diagrams

Source: `termchat` vault (21 notes): `01-Overview/Architecture.md`, `01-Overview/Repository-Map.md`, `04-Development/Delivery-and-Deploy.md`, `04-Development/Server-Internals.md`, `04-Development/Concurrency.md` + live code map.

Module: `termchat` (go 1.26.3). No root package. Mains in `cli/` and `server/cmd/server/`.

## 1. Package diagram

```mermaid
packageDiagram
package termchat {
  package shared {
    + protocol.go : Message, Reaction, UserInfo, RoomInfo
    + room.go : GenerateRoomCode, NormalizeRoomCode
    + validation.go : IsValidNickname, IsValidRoomCode
    + media.go : EncodeMediaFrame, ParseMediaFrame
    + constants.go
  }
  package server {
    + server.go : StartServer, Stop
    + websocket.go : readPump, writePump, broadcastToRoom
    + room.go : Room, roomsMutex
    + client.go : Client, trySend, close
    + media.go : token mint/burn, relay
    + bootstrap.go : render, binaryHandler
  }
  package cli <<main>> {
    + main.go, app.go, tui.go
    + websocket.go, media.go
    + lan.go, discover.go
    + audio.go, video.go
    + config.go, theme.go, commands.go
  }
  package "server/cmd/server" <<main>> {
    + main.go
  }
}
"server/cmd/server" --> server : imports
server --> shared : imports
cli --> shared : imports
cli ..> server : test-only
note right of shared : leaf, no internal imports
```

Deps are acyclic: `cmd/server -> server -> shared`, `cli -> shared`.

## 2. Component diagram

```mermaid
componentDiagram
component [CLI Binary\ncli/main] as CLI
component [TUI\ntui, app, commands] as TUI
component [WS Conn\nwebsocket, discover] as CONN
component [Media AV\nmedia, audio, video] as AV
component [LAN Beacon\nlan 224.0.0.167:9999] as LAN
component [Server Binary\nserver/cmd/server] as SRV
component [HTTP Mux\n9 routes] as MUX
component [Room Registry\nroom.go + roomsMutex] as ROOMS
component [WS Hub\nreadPump/writePump] as HUB
component [Media Hub\nmedia.go relay] as MHUB
component [Bootstrap\nembed sh/ps1 + /bin] as BOOT
component [Shared Kernel\nprotocol + validation] as SHARED
component [Caddy\nreverse_proxy :8080] as CADDY

TUI -- CONN : drives
CONN -- AV : /vc token
LAN -- CONN : peer list
CLI -- TUI : contains
CLI -- CONN : contains
CLI -- AV : contains
CLI -- LAN : contains
CONN --> MUX : GET /ws, /discover
AV --> MUX : WS /media
MUX -- HUB : upgrade
MUX -- MHUB : dial
MUX -- BOOT : /{room}, /bin/*
HUB -- ROOMS : get-or-create
MHUB -- ROOMS : peers-only relay
SRV -- MUX : contains
SRV -- ROOMS : contains
SRV -- HUB : contains
SRV -- MHUB : contains
SRV -- BOOT : contains
CLI ..> SHARED : GenerateRoomCode, Message
SRV ..> SHARED : Message, RoomInfo
CADDY --> SRV : websocket:8080
```

Interfaces: `/ws` chat, `/media` voice/video (6-byte framed PCM+JPEG), `/discover` ONLINE ROOMS, `/{room}` + `/bin/{binary}` bootstrap.

## 3. Deployment diagram

```mermaid
flowchart TB
  subgraph GH[GitHub]
    TAG[tag cli-v*] --> CLIW[cli.yml\n7 binaries + checksums\nldflags Version + DefaultWS]
    CLIW --> REL[Release\nbinaries + termchat-checksums.txt]
    CLIW --> AURW[aur.yml\nAUR sync via SSH]
    CLIW --> WSIMG[websocket.yml\nGHCR termchat-websocket:tag/latest/sha]
    CLIW --> NPMW[npm.yml OIDC as cli.yml\n8x @sacredcat/termchat*]
    CI[ci.yml + gate.yml\ntody/gofmt/vet + race + fuzz] -. gate .-> TAG
  end
  subgraph VPS[VPS termchat.sacred99.online]
    CADDY[Caddy :80/:443\nCaddyfile reverse_proxy]
    WSC[websocket container\ndebian nonroot 10001 :8080\nWS_HOST WS_PORT\nPUBLIC_BASE_URL GITHUB_REPO]
    WT[watchtower --interval 30]
    CADDY --> WSC
    WT -. pull :latest .-> WSC
  end
  WSIMG --> WSC
  subgraph USER[User machine Linux/macOS/Win]
    CURL[curl BASE_URL/room | sh\nbootstrap.sh/ps1\nRoom BaseURL Version]
    BIN[~/.termchat cache\ntermchat-os-arch\n--server wss://.../ws]
    TUI2[CLI TUI + LAN UDP 224.0.0.167:9999]
    CURL --> BIN --> TUI2
  end
  WSC -. renders .-> CURL
  TUI2 --> CADDY
  REL -. fallback download .-> CURL
  NPMW -. npm i -g .-> BIN
```

Ports/env: `WS_PORT 8080`, `PUBLIC_BASE_URL` drives `--server` + `/bin` redirect, `/healthz` check, idle 30m / rate 5/s enforced server-side.
