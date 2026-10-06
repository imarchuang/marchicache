# marchicache

Educational Redis-inspired in-memory cache in Go. Memory-first dict;
disk (AOF) is for restart, not the read path. See [PLAN.md](PLAN.md).

## Run

```bash
go run ./cmd/marchicache -addr=:6380
```

```bash
curl -X PUT localhost:6380/kv/session -d token-abc
curl localhost:6380/kv/session
curl localhost:6380/healthz
curl -X DELETE localhost:6380/kv/session
```

Flags: `-addr=:6380`, `-dataDir` (reserved).

## Docker

```bash
docker build -t marchicache .
docker run --rm -p 6380:6380 marchicache
```
