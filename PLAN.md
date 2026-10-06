# marchicache — Redis-inspired in-memory cache MVP

Educational cache in Go. Same spirit as other `marchi*` repos: **one
learning goal per slice**, inspectable AOF, tests for expire and crash
replay.

**Not Redis / not KeyDB.** We borrow the **in-memory dict, TTL, AOF
(and optional snapshot), a few core types** — not the full command
surface, Cluster bus, Sentinel, or modules.

This fills the Key Technologies gap: every other store you built is
**disk-first**; Redis is **memory-first**, with disk as a recovery log.

---

## Learning goal

1. The data is a **hash table in RAM**; disk is for *restart*, not for
   the read path.
2. **TTL** is a second index (expiry heap / lazy + active deletion), not
   a field you scan on every GET.
3. **AOF** = append commands (or rewritten values); fsync policy
   (`always` / `everysec` / `no`) is the durability knob.
4. **RDB / rewrite** is a point-in-time dump so AOF does not grow forever.
5. Values have **types** (string, hash, list, zset subset); commands are
   type-checked, not schemaless JSON blobs.
6. Single-threaded *command execution* (one goroutine mutating the dict)
   is a product choice: no per-key locks, pipelining still works.

**Pass bar:** SET with EX, kill process before TTL fires, restart from AOF,
key is still there with remaining TTL ≈ original; expire then GET miss.
Optional: speak enough **RESP** that `redis-cli` can GET/SET.

---

## Concepts we keep (and drop)

| Redis | marchicache v0 | Deferred |
|---|---|---|
| String GET/SET/DEL | yes | SET NX/XX/GET, GETEX |
| EXPIRE / TTL / lazy + active expire | yes | keyspace notifications |
| HASH HSET/HGET/HGETALL | yes | HSCAN |
| LIST or ZSET | pick **one** extra type (ZSET if you care about rankings) | streams, bitmaps, geo |
| AOF | `appendonly.aof`, fsync everysec default | AOF timestamp, mixed RDB preamble |
| BGREWRITEAOF / snapshot | rewrite compact AOF on demand | fork/COW (use rewrite in-process) |
| RESP TCP | slice for redis-cli | Cluster, Sentinel, pub/sub |
| Eviction | `maxmemory` + `allkeys-lru` (approx) | LFU, volatile-* |
| Single writer | mutex or dedicated loop | I/O threads like Redis 6 |

**Non-goals:** Redis Cluster (MOVED/ASK), replication + replica-async,
Lua, modules, ACL, TLS.

Hash slots belong in a *later* distributed slice; v0 is one process.

---

## Core loop

```text
Client  --RESP or HTTP-->  command goroutine
                              |
                              |  expire lazy check
                              |  mutate dict (+ ttl index)
                              |  append AOF  (maybe fsync)
                              v
                            reply
```

Periodic ticker: **active expire** samples random keys with TTL.

---

## On-disk layout

```text
{dataDir}/
  appendonly.aof         # RESP or JSONL commands, fsynced per policy
  dump.rdb.json          # optional snapshot: {k, type, value, expireAt}
  rewrite.tmp            # AOF rewrite in progress then rename
```

AOF record (JSONL is fine if RESP rewrite is later):

```text
{"op":"SET","key":"a","value":"1","expireAt":0}
{"op":"DEL","key":"a"}
```

On open: load snapshot if present, then replay AOF after snapshot offset.

---

## API

**HTTP (tests + curl):**

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | keys, aof bytes, fsync policy |
| PUT | `/kv/{key}` | body = string; `?ex=seconds` |
| GET | `/kv/{key}` | 200 / 404 |
| DELETE | `/kv/{key}` | DEL |
| PUT | `/hash/{key}/{field}` | HSET |
| GET | `/hash/{key}` | HGETALL |
| POST | `/expire/{key}?ex=` | EXPIRE |
| GET | `/ttl/{key}` | TTL |
| POST | `/rewrite` | compact AOF |

**RESP (slice):** TCP `-redisAddr=:6379` implementing PING, GET, SET, DEL,
EXPIRE, TTL, HSET, HGET, HGETALL, COMMAND maybe stub.

Flags: `-addr=:6380` (HTTP), `-redisAddr=:6379`, `-dataDir`,
`-appendfsync=everysec|always|no`, `-maxmemory=0`.

---

## MVP slices

### Slice 0 — skeleton
HTTP GET/SET/DEL, in-memory map, no TTL, no AOF.

### Slice 1 — TTL
`EX` / EXPIRE / TTL; lazy expire on GET; active expire ticker.
Test: fake clock, key vanishes after T.

### Slice 2 — AOF
Every mutate appends; reopen replays; `everysec` vs `always`.
Test: SET, kill without clean shutdown (`always` survives).

### Slice 3 — HASH + one more type
HSET/HGET/HGETALL. Optionally ZADD/ZRANGE (skip-list or sorted slice).
Test: type mismatch SET then HGET → error.

### Slice 4 — AOF rewrite + maxmemory LRU
Rewrite dumps current dict; rename atomically.
Evict when over `maxmemory` (sample LRU).
Test: rewrite file smaller; GET after rewrite; eviction of cold keys.

### Slice 5 — RESP
Enough for `redis-cli -p 6379 SET/GET`.
Demo in README.

---

## Demo (graduation)

```bash
go run ./cmd/marchicache -dataDir=./data -appendfsync=always
curl -X PUT 'localhost:6380/kv/session?ex=60' -d token-abc
# kill -9; restart
curl localhost:6380/kv/session          # still there
curl localhost:6380/ttl/session         # positive TTL
```

Optional:

```bash
redis-cli -p 6379 SET foo bar EX 10
redis-cli -p 6379 GET foo
```

---

## Relation to siblings

| Project | Contrast |
|---|---|
| **marchibtree / marchisql** | disk pages / MVCC; Redis never walks a heap for GET |
| **marchiq** | Redis Stream is a log; v0 skips streams (Kafka is marchiq) |
| **marchiraft** | Redis replication is *not* Raft; don’t pretend it is |
| **marchidynamo** | durable quorum KV vs cache with TTL and eviction |

Start at **slice 0** on `feat/skeleton`.
