# HoloNet

**SNMP trap → messaging bridge.** HoloNet receives SNMP traps from network
devices (initial target: Sophos SFVH / SFOS), classifies each event into a
priority level, applies flood control, and dispatches notifications to
configurable channels (Telegram, WhatsApp, webhook, and anything
[Shoutrrr](https://github.com/containrrr/shoutrrr) supports).

> Status: **Slice 1 complete** — the end-to-end v2c → Telegram spine. SNMPv3,
> the rule engine, flood control, the web UI, and Prometheus metrics land in
> the following slices.

## Pipeline

```
UDP :1162 → Trap Sink (v2c) → Decoder (OID→event) → Classify → Dispatch → Persist (SQLite)
```

- **Trap sink** (`internal/snmp`) — gosnmp `TrapListener`, recover-guarded so a
  malformed packet can never crash the process. v2c traps are accepted only when
  their community string matches an enabled, sealed record; unknown communities
  are dropped and counted.
- **Decoder** (`internal/decode`) — resolves `snmpTrapOID.0` against the OID map,
  assigns a default severity (unmapped OIDs take the configured unknown-event
  default and are flagged for later mapping), and composes a human message.
- **Notifiers** (`internal/notify`) — a `Notifier` per channel kind. Slice 1
  ships the Shoutrrr notifier; dispatch fans out concurrently with per-channel
  timeout and bounded retry.
- **Store** (`internal/store`) — single SQLite file (`modernc.org/sqlite`,
  CGO-free), WAL mode, embedded migrations. Source of truth for all operational
  config.
- **Secret sealing** (`internal/crypto`) — every credential (community strings,
  channel config) is AES-256-GCM sealed at rest; no plaintext secret is written
  to the database.

## Build

```sh
go build ./...
go test ./...
go build -o holonet ./cmd/holonet
```

`CGO_ENABLED=0` throughout (pure-Go SQLite), so binaries cross-compile cleanly.

## Run

HoloNet needs a master key for secret sealing and a database path. Everything
else (bind address, channels, communities) lives in the database.

```sh
# 1. Bootstrap a v2c community and a Shoutrrr (e.g. Telegram) channel:
export HOLONET_MASTER_KEY="$(openssl rand -base64 32)"
./holonet --db-path ./holonet.db --add-community public
./holonet --db-path ./holonet.db \
  --add-shoutrrr "telegram=telegram://<bot-token>@telegram?chats=@<channel>"

# 2. Start the daemon (listens on the DB-configured bind address, default 0.0.0.0:1162):
./holonet --db-path ./holonet.db
```

Send a test trap with net-snmp:

```sh
snmptrap -v2c -c public 127.0.0.1:1162 '' 1.3.6.1.6.3.1.1.5.3 \
  1.3.6.1.4.1.2604.5.1.1 s "Interface Port2 link is down"
```

### CLI flags

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `--db-path` | `HOLONET_DB_PATH` | `/data/holonet.db` | SQLite database file |
| `--master-key` | `HOLONET_MASTER_KEY` | *(required)* | AES-GCM key for sealing secrets |
| `--http-addr` | `HOLONET_HTTP_ADDR` | `:8080` | Web/API listen address (Slice 2) |
| `--log-level` | `HOLONET_LOG_LEVEL` | `info` | `debug` / `info` / `warning` / `error` |
| `--version` | | | Print version and exit |
| `--add-community` | | | Seal + insert a v2c community, then exit |
| `--add-shoutrrr` | | | Insert a Shoutrrr channel (`name=url`), then exit |

Precedence is `ENV` → `--flag` → built-in default. The SNMP **bind address** is
stored in SQLite (default `0.0.0.0:1162`), not passed as a flag.

## Built-in OID seed

The RFC-standard generic notifications are seeded on first run:

| OID | Event | Default severity |
|-----|-------|------------------|
| `1.3.6.1.6.3.1.1.5.1` | coldStart | High |
| `1.3.6.1.6.3.1.1.5.2` | warmStart | Medium |
| `1.3.6.1.6.3.1.1.5.3` | linkDown | High |
| `1.3.6.1.6.3.1.1.5.4` | linkUp | Low |
| `1.3.6.1.6.3.1.1.5.5` | authenticationFailure | High |

Sophos enterprise OIDs are imported from the SFOS MIB (a Slice 3 walkthrough),
not hardcoded.

## License

Apache-2.0.
