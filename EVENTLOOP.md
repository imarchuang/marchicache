# Event loop

Redis `ae.c` is not async magic. It is one thread that waits for the kernel
to name ready sockets, then **read → parse → execute → write** without
parking the process.

## epoll/kqueue vs goroutine-per-connection

| | kqueue/epoll reactor | `go handleConn` + mutex |
|---|---|---|
| Idle clients | file descriptors + a small client struct | OS stacks + scheduler work |
| Dict | no lock; only the loop mutates it | `sync.Mutex` on every GET |
| Command order | pipelined commands run one after another on the loop | easy to race across a connection |
| Slow command | `KEYS *` or `appendfsync always` stalls **every** client | one conn stalls; others still contend on the mutex |
| What Go already has | the runtime netpoller | still not *your* single-threaded dict |

marchicache v0 uses **kqueue on Darwin** and **epoll on Linux**. Client
fds are non-blocking. Partial reads live in `querybuf`; partial writes in
`outbuf` with a writable watch when `write` returns EAGAIN.

HTTP is allowed to `Do()` work onto that same loop through a channel (plus
a wakeup pipe). RESP is not “start a goroutine per socket and lock the
map.”

## Pipelining

`SET a 1\r\nGET a\r\n` is two commands in one `querybuf`. The loop parses
and executes them serially, then flushes replies. Throughput without extra
threads.

## Why `KEYS` is banned in production

A command runs to completion on the loop. `KEYS *` walks the whole dict
while no other client is accepted, read, or responded to. Same class of
footgun as `appendfsync always` on a large write. Use targeted GET/SET or
a bounded scan; do not stall the reactor.

Redis 6 I/O threads only copy bytes. Execution stays single-threaded.
We do not copy that.
