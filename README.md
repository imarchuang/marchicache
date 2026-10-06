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

HASH: `PUT /hash/{key}/{field}`, `GET /hash/{key}` (HGETALL), `GET /hash/{key}/{field}`. ZSET: `PUT /zset/{key}/{member}?score=`, `GET /zset/{key}`. SET then HGET returns WRONGTYPE.

Flags: `-addr=:6380`, `-redisAddr=:6379`, `-dataDir=./data`, `-appendfsync=everysec|always|no`, `-maxmemory=0`.

RESP is served on **one kqueue/epoll reactor goroutine** (see [EVENTLOOP.md](EVENTLOOP.md)). `redis-cli -p 6379 SET foo bar EX 10` works.

```bash
redis-cli -p 6379 PING
redis-cli -p 6379 SET foo bar EX 10
redis-cli -p 6379 GET foo
```

AOF is JSONL in `{dataDir}/appendonly.aof`. `always` fsyncs each mutate so a kill without flush still replays. `everysec` batches fsync on a ticker. `POST /rewrite` compact-dumps the live dict. `maxmemory` enables approximate allkeys-LRU.

## Docker

```bash
docker build -t marchicache .
docker run --rm -p 6380:6380 marchicache
```
