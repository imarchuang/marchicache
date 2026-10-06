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
6. **Event loop (ae):** one thread multiplexes thousands of sockets
   (`epoll`/`kqueue`), then **read → parse → execute → write** on that
   same thread. High traffic ≠ one OS thread per connection. The dict
   has **no lock** because only the loop mutates it.

**Pass bar:** SET with EX, kill process before TTL fires, restart from AOF,
key is still there with remaining TTL ≈ original; expire then GET miss.
RESP path: N connections pipelining SET/GET; race detector shows dict
touched from one goroutine; `redis-cli` works.

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
| RESP TCP | yes, on the event loop | Cluster, Sentinel, pub/sub |
| Event loop | one reactor: poll + execute | Redis 6 I/O threads (read/write only) |
| Eviction | `maxmemory` + `allkeys-lru` (approx) | LFU, volatile-* |

**Non-goals:** Redis Cluster (MOVED/ASK), replication + replica-async,
Lua, modules, ACL, TLS.

Hash slots belong in a *later* distributed slice; v0 is one process.

---

## Core loop

HTTP tests may use a serializer channel. The **Redis learning path** is
the RESP reactor (see below), not `go handleConn` + `sync.Mutex`.

```text
                    kqueue / epoll_wait
                              |
              +---------------+---------------+
              | ready fd      | timeout       |
              v               v
        read querybuf    time events (active expire)
        parse RESP
        processCommand   ← only here the dict moves
        try write reply  (buffer if EAGAIN, watch writable)
```

Periodic **time event**: active expire samples random keys with TTL
(same loop, not a second writer).

---

## Event loop — what to actually learn

Redis `ae.c` is not “async magic.” It is:

1. **I/O multiplexing:** the kernel says which fds are readable/writable.
   10k idle clients cost **file descriptors**, not 10k stacks.
2. **Non-blocking sockets:** `read`/`write` never park the process; partial
   buffers live on the client struct (`querybuf` / `buf`).
3. **Serialized execution:** `processCommand` runs to completion on the
   loop. Pipelining = many commands from one `querybuf`, still one after
   another. No dict mutex.
4. **Time events:** expire, cron, `everysec` AOF, next to file events in
   the same `aeMain` loop.
5. **The cost:** a slow command (`KEYS *`) or `appendfsync always` stalls
   **every** client. That is the whole single-thread tradeoff.

**Go trap:** `net.Listen` + `go func()` per conn is **not** this model.
The Go runtime already has a netpoller, but **your** dict would need a
mutex and command order across a connection is easy to get wrong.

**marchicache v0 must implement a real reactor for RESP:**

- One goroutine owns `dict`, TTL index, AOF.
- That goroutine `epoll_wait`/`kqueue` (or `unix.Kevent` on Darwin) on
  the listen fd + client fds.
- Accept / read / execute / write happen there. No `mu.Lock` on the dict.
- Tests: `-race` with 100 conns × pipelined SET; optional assertion that
  dict methods are only called from the loop goroutine id.

Acceptable **non-goal:** copying Redis 6 I/O threads. Mentally: those
threads only copy bytes; execution stays single-threaded.

HTTP can stay “naive” (goroutine + send command to the loop via a
bounded channel). That still serializes execution but **does not** teach
multiplexing — hence RESP+reactor is mandatory, not optional polish.

Write `EVENTLOOP.md` in the reactor slice: epoll vs goroutine-per-conn
table, pipelining, and why `KEYS` is banned in prod.

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

### Slice 5 — RESP + event loop (the Redis I/O thesis)
TCP `-redisAddr=:6379`. **One reactor goroutine:** kqueue/epoll, non-blocking
client fds, RESP parse, execute on the loop, buffered writes.
No mutex on `dict`. Pipelining must work (`SET a 1\r\nGET a\r\n`).
`EVENTLOOP.md` + test: 100 connections, `-race` clean, command order per conn.
`redis-cli` GET/SET/EXPIRE.

This slice is not “also speak RESP.” It is **how one thread takes a lot of
traffic.** Do not ship RESP as goroutine-per-connection.

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
