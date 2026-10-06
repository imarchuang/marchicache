# marchicache

Educational Redis-inspired in-memory cache in Go. Memory-first dict;
disk (AOF) is for restart, not the read path. See [PLAN.md](PLAN.md).

## Run

```bash
go run ./cmd/marchicache -addr=:6380
```

```bash
curl -X PUT 'localhost:6380/kv/session?ex=60' -d token-abc
curl localhost:6380/kv/session
curl localhost:6380/ttl/session
curl -X POST 'localhost:6380/expire/session?ex=30'
curl localhost:6380/healthz
curl -X DELETE localhost:6380/kv/session
```

TTL uses lazy expire on GET plus an active expire ticker (samples keys). Redis semantics: TTL `-2` missing, `-1` no expire.

Flags: `-addr=:6380`, `-dataDir` (reserved).

## Docker

```bash
docker build -t marchicache .
docker run --rm -p 6380:6380 marchicache
```
