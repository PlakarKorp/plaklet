# plaklet

`plaklet` is a small, single-shot task executor built on
[kloset](https://github.com/PlakarKorp/kloset). It reads one task description as
JSON on stdin, runs it (backup, check, …) against kloset connectors linked into
the binary, and streams the result back as JSON on stdout.

It is the execution engine that a driver spawns to actually do work — for
example [plakar-edge](https://github.com/PlakarKorp/plakar-edge), which forwards
tasks from a control plane to a remote network. plaklet itself is
control-plane-agnostic: it has no dependency on plakman, only on kloset and a
set of built-in connectors.

## Protocol

plaklet reads a single `ExecPayload` object on stdin and writes a stream of
`ExecReply` objects (one JSON object per line) on stdout, ending with a terminal
`success` or `failure`. The shapes are defined in [`protocol.go`](protocol.go).

```jsonc
// stdin — one ExecPayload
{
  "op": "backup",
  "task_config": { "tags": "nightly", "ignore": "*.tmp" },
  "source": {                       // resolved connector configuration
    "integration": { "name": "fs", "version": "1.1.2" },
    "fields": [ { "key": "location", "val": "fs:///data" } ]
  },
  "target": {
    "integration": { "name": "fs", "version": "1.1.2" },
    "fields": [ { "key": "location", "val": "fs:///backups/store" } ]
  }
}
```

```jsonc
// stdout — a stream of ExecReply, terminal reply last
{"type":"report","report":{ "type":"backup", "backup": { … } }}
{"type":"success"}
```

Connector secrets are expected to be **already resolved** into literal field
values before the payload reaches plaklet.

## Operations

| Op | Needs | Description |
|----|-------|-------------|
| `backup` | source + target | Snapshot a source into a store |
| `check`  | source (a store) | Verify every snapshot in a store |

Other operations (sync, restore, prune, maintenance) follow the same pattern and
can be added in their own files.

## Connectors

The backends are linked in-process (see [`connectors.go`](connectors.go)) — the
same set the public `plakar` CLI ships: `fs`, `http`, `ptar`, `stdio`, `tar`.
Add an integration subpackage import to support more.

## Build & run

```sh
go build -o plaklet .

plaklet -cache /var/cache/plaklet -quiet < payload.json
```

| Flag | Default | Meaning |
|------|---------|---------|
| `-cache` | *(required)* | Cache directory (pebble state cache) |
| `-cpu` | GOMAXPROCS-1 | Number of CPUs to use |
| `-concurrency` | NumCPU | Max backup/scan worker concurrency |
| `-quiet` | false | Quiet |
| `-pkg` | | Reserved; connectors are linked in-process |
