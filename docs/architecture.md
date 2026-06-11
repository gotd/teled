# Architecture: teled

Status: Draft
Date: 2026-06-11

This document describes the internal architecture of `teled` as a single-instance
Telegram server. It assumes the scope and technology decisions in
[proposal.md](proposal.md).

## Layered overview

```
                      ┌─────────────────────────────────────────────┐
   MTProto client ───▶│  Transport / MTProto  (internal/mtproto)     │
   (gotd/td, tdlib)   │  custom server over gotd/td transport,       │
                      │  exchange, crypto; obfuscation, key exchange,│
                      │  encryption, acks, salts — persistent keys   │
                      └───────────────┬─────────────────────────────┘
                                      │ decrypted TL request + Session
                      ┌───────────────▼─────────────────────────────┐
                      │  RPC routing  (tg.ServerDispatcher)          │
                      │  781 OnX handlers; we register a subset      │
                      └───────────────┬─────────────────────────────┘
                      ┌───────────────▼─────────────────────────────┐
                      │  Handlers (internal/rpc)                     │
                      │  decode → call service → encode TL response  │
                      └───────────────┬─────────────────────────────┘
                      ┌───────────────▼─────────────────────────────┐
                      │  Services (internal/service)                 │
                      │  Auth, User, Contact, Dialog, Message, Media │
                      │  Update — business logic, transactions       │
                      └───┬───────────────┬───────────────┬─────────┘
                          │               │               │
              ┌───────────▼──┐  ┌──────────▼─────┐  ┌──────▼────────────┐
              │ DB (Postgres)│  │ ObjectStore    │  │ Queue (River)     │
              │ pgx+squirrel │  │ S3-shaped / FS │  │ media, cleanup    │
              └──────────────┘  └────────────────┘  └───────────────────┘
                          ▲
              ┌───────────┴───────────────┐
              │ Updates engine            │  pushes via internal/mtproto Conn
              │ (internal/updates)        │  session registry: user → Session
              └───────────────────────────┘
```

Dependency rule: handlers depend on service interfaces; services depend on
storage interfaces (`DB`, `ObjectStore`, `Queue`, `AuthKeyStore`); storage impls
live in `internal/*` and depend on nothing above them. Domain models live in the
root `teled` package (MVC-style, as in lilith).

## Package layout

```
teled.go                       root package: domain models + core interfaces
                               (DB, ObjectStore, Queue, AuthKeyStore, User,
                               Message, Dialog, File, Update, ...)
cmd/teled/main.go              entrypoint (cobra), wires everything

internal/
  cmd/                         CLI / config wiring (exists today)
  mtproto/                     custom MTProto server (replaces tgtest):
                               listener.go  accept loop over transport.Listen
                               conn.go      per-connection serve loop, state
                               exchange.go  server-side DH (gotd/td exchange)
                               handle.go    decrypt, service msgs, dispatch RPC
                               send.go      encrypt + write, push updates
                               registry.go  live conns + session registry
  server/                      bootstrap: build mtproto server, register
                               dispatcher, run migrations, lifecycle
  rpc/                         ServerDispatcher handlers grouped by domain:
                               auth.go users.go contacts.go dialogs.go
                               messages.go upload.go updates.go help.go
                               account.go; plus mapping helpers (TL <-> model)
  service/                     business logic services
                               auth.go user.go contact.go dialog.go
                               message.go media.go update.go
  updates/                     update sequence engine + session registry
  db/                          PostgreSQL implementation
    open.go migrations.go db.go
    auth_key.go session.go user.go contact.go dialog.go message.go
    file.go update.go
    _migrations/*.sql
    SCHEMA.md
  objstore/                    ObjectStore implementations
    fs.go                      local filesystem (v1)
    (s3.go)                    future S3-compatible backend
  queue/                       River client setup + workers
    queue.go workers/*.go
  mock/                        generated mocks (moq)
docs/
  proposal.md architecture.md schema.md
```

## Core interfaces (root `teled` package)

Defined in the root package so impls in `internal/*` satisfy them, mirroring
lilith's `DB` / `FileStore` placement.

```go
// ObjectStore is an S3-shaped blob store. Keys are opaque, sharded by impl.
type ObjectStore interface {
    Put(ctx context.Context, key string, r io.Reader, size int64, opt PutOptions) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    // GetRange returns [off, off+length) — required for upload.getFile chunking.
    GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error)
    Stat(ctx context.Context, key string) (ObjectInfo, error)
    Delete(ctx context.Context, key string) error
}

// AuthKeyStore persists MTProto auth keys (see "Auth keys and sessions").
type AuthKeyStore interface {
    Save(ctx context.Context, k AuthKey) error
    Get(ctx context.Context, keyID [8]byte) (AuthKey, bool, error)
    Delete(ctx context.Context, keyID [8]byte) error
}

// DB is the persistence port; the concrete impl lives in internal/db.
type DB interface {
    Ready(ctx context.Context) error
    // ... domain repositories, see schema.md
}

// Queue enqueues background jobs; the concrete impl wraps River.
type Queue interface {
    Enqueue(ctx context.Context, job Job) error
}
```

Interfaces are mocked with `moq` (as in lilith) into `internal/mock` via
`//go:generate` directives, so services are unit-testable without Postgres.

## Transport / MTProto layer (`internal/mtproto`)

We **do not** use `tgtest.Server`. Instead `internal/mtproto` is a custom server
that composes the same gotd/td low-level packages tgtest itself builds on. The
split between reused cryptography and our orchestration:

**Reused from gotd/td unchanged** (the hard, security-critical parts):

- `transport` — `transport.Listen`/`ListenCodec`, obfuscated + websocket
  listeners, codec detection, framing.
- `exchange` — server-side Diffie-Hellman: `exchange.NewExchanger(conn, dc).
  Server(key).Run(ctx)` returns the negotiated `crypto.AuthKey` + server salt.
- `crypto` — MTProto v2 message cipher (encrypt/decrypt), auth-key/message-key
  derivation.
- `proto` / `mt` / `bin` / `mtproto` — message containers, service messages,
  `msg_id`/`seq_no` sources, salt types, encoding.
- `tg.ServerDispatcher` + `tg` — the TL layer.

**Implemented by us** (the ~700 lines that were tgtest-specific and in-memory):

- `listener.go` — accept loop; one `serveConn` goroutine per connection.
- `conn.go` — per-connection serve loop: read frame, peek 8-byte `auth_key_id`,
  branch:
  - zero → run key exchange, **persist** the resulting key via `AuthKeyStore`,
    register the session;
  - known (in registry **or** loadable from `AuthKeyStore`) → decrypt + handle;
  - unknown → send `codec.CodeAuthKeyNotFound`.
- `handle.go` — decrypt with `crypto.Cipher`, unwrap containers/gzip, process
  MTProto service messages (acks, `bad_msg_notification`, `msgs_ack`,
  `future_salts`, ping/pong, `new_session_created`), and dispatch the RPC body to
  `tg.ServerDispatcher`.
- `send.go` — encrypt + write; helpers `SendResult` / `SendUpdates` / `SendAck`
  (the surface tgtest exposed), used by handlers and the updates engine.
- `registry.go` — live connections + sessions (below).

Multi-DC: v1 runs a **single DC**, but the listener/key/DC id are parameterised
so additional DCs are added without touching the loop. `help.getConfig` returns
our DC list so clients connect back to us.

`internal/server` owns construction: build the dispatcher, register handlers
(`internal/rpc`), start the listener. The current `internal/cmd/app.go`
(`application` + `OnMessage`/`Fallback` over `tgtest`) is rewritten against the
new server; its handler-registration shape carries over.

> Correctness note: owning the loop means we own MTProto service-message
> correctness. `tgtest/handle.go` and the client-side `mtproto/handle_*.go` are
> the reference; conformance is asserted by the end-to-end gotd/td client test.

### Auth keys and sessions

Because we own the connection loop, auth keys and sessions **persist by design**:

- On key exchange we `AuthKeyStore.Save` the new `crypto.AuthKey` (`auth_keys`
  table). On every subsequent connection the `auth_key_id` is resolved from the
  in-memory registry, falling back to `AuthKeyStore.Get` — so a client reconnects
  with its existing key after a server restart, no re-exchange.
- An MTProto `Session{ID, AuthKey}` is bound to a logged-in `user_id` after
  `auth.signIn`; the binding is persisted in `sessions` and mirrored in the
  in-memory registry for push.
- The registry holds *live* connections for push; `auth_keys`/`sessions` hold the
  durable truth. The registry is rebuilt lazily from the store as clients
  reconnect.

## RPC handlers (`internal/rpc`)

One file per TL namespace. Each handler:

1. receives the decoded TL request and the `tgtest.Request` (carrying
   `Session`, `RequestCtx`);
2. resolves the calling `user_id` from the session registry;
3. calls a service method with plain model arguments;
4. maps the model result back to TL types and returns the `bin.Encoder`.

TL ↔ model mapping lives beside the handlers (not in services) so services never
import `tg`. This keeps business logic protocol-agnostic and testable.

The existing stub handlers in `internal/cmd/app_handlers.go` are migrated here
incrementally; handlers not yet backed by a service keep returning stubs so the
server stays bootable at every milestone.

## Services (`internal/service`)

Plain Go, depend only on root interfaces. Each owns one domain and the
transaction boundary for its writes.

- **AuthService**: `SendCode` (create `phone_codes`, hand code to a `CodeSink`),
  `SignIn`/`SignUp` (validate, create/get `users`, bind session→user),
  `LogOut`.
- **UserService**: self profile, `GetUsers`, `GetFullUser`, username resolution.
- **ContactService**: import/get/delete contacts, `resolveUsername`.
- **DialogService**: dialog list, pinned, read state, drafts.
- **MessageService**: `SendMessage`/`SendMedia`/`EditMessage`/`DeleteMessages`,
  `GetHistory`, `ReadHistory`. Owns the per-account message-id allocation and,
  in the same transaction, calls UpdateService to append to the update log.
- **MediaService**: multipart upload staging, finalize→`ObjectStore.Put` +
  `files` row, download via `ObjectStore.GetRange`, file references.
- **UpdateService**: allocate `pts`, persist `updates_log`, build
  `updates.Difference`, and notify the in-process updates engine to push.

### Write + update atomicity

A message send is one PostgreSQL transaction:

```
BEGIN
  insert canonical message            -> messages
  insert sender + recipient refs      -> message_refs (per-account local ids)
  allocate next pts for each affected user
  append update rows                  -> updates_log (pts, update payload)
  bump per-user state                 -> update_state
COMMIT
  → hand affected (user_id, pts, update) to updates engine for live push
```

Push happens *after* commit; if the recipient is offline the durable
`updates_log` lets them catch up via `getDifference`. Push delivery is
best-effort and never blocks the transaction.

## Updates engine (`internal/updates`)

Two responsibilities:

1. **Session registry** — in-memory `map[userID][]Session`. Sessions register on
   connect (after auth binding) and deregister on disconnect. For each new
   committed update affecting a user, the engine calls the `internal/mtproto`
   server's `SendUpdates(ctx, session, ...)` for that user's live sessions. This
   map is the single component that ties us to one instance (see below).
2. **Difference reconciliation** — `updates.getState` returns the user's current
   `(pts, qts, seq, date)`; `updates.getDifference` returns `updates_log` rows
   with `pts > client_pts`. If the gap exceeds a threshold (or rows were pruned)
   it returns `updates.differenceTooLong`, prompting a state resync.

### Update sequence

Each account has a monotonic `pts`. In a single-instance monolith we allocate it
inside the send transaction. Two viable mechanisms (open question in proposal):

- **Row-locked counter** in `update_state` (`SELECT pts FROM update_state WHERE
  user_id=? FOR UPDATE; ... UPDATE`). Simple, transactional, ordering guaranteed.
  Preferred for v1.
- A per-user PostgreSQL **sequence**. Cheaper under contention but harder to keep
  perfectly gapless alongside the log; not needed at single-instance scale.

`qts` (secret chats) and channel `pts` are out of scope for v1 but the
`update_state` row reserves columns for them.

## Object storage (`internal/objstore`)

S3-shaped interface (above), with a local-filesystem implementation for v1.

- **Key scheme**: content-addressed, `sha256` hex, sharded
  `ab/cd/abcdef...` to avoid huge directories. Stored under a configured base
  dir. `files.object_key` holds the key.
- **Ranged reads**: `upload.getFile` requests `(offset, limit)` aligned to
  Telegram's part rules (≤ 1 MB, offset % 1 KB == 0). The FS impl uses
  `os.File.Seek` + `io.LimitReader`; the future S3 impl uses a `Range` header —
  the interface is identical.
- **Upload staging**: `upload.saveFilePart`/`saveBigFilePart` deliver parts out
  of order. Parts are staged (temp dir keyed by `file_id`, or an
  `upload_parts` table for crash safety) and assembled on `sendMedia`. On
  finalize the assembled blob is hashed, `Put` to the store, and a `files` row
  created. The staging area is then cleared.
- **File references**: `file_reference` (opaque bytes) is issued per `files` row;
  `upload.getFile` validates it. Regeneration is a later concern.

## Background queue (`internal/queue`, River)

River runs **in-process** workers against the same PostgreSQL database (its own
migration set, run alongside ours). Jobs:

- **Media processing**: thumbnail/`PhotoSize` generation, dimension/mime probing
  after upload. Keeps `sendMedia` latency low.
- **Delivery retry**: re-attempt push for sessions that errored (the durable log
  is the source of truth; this just reduces reconnect latency).
- **Cleanup (periodic)**: expire `phone_codes`, prune `updates_log` below a
  retention horizon, GC orphaned objects/`files`.

For v1 DMs the fan-out is two participants, so message delivery itself is inline;
River is for asynchronous/retryable/periodic work, not the hot send path. When
groups/channels arrive, broadcast fan-out becomes a River job.

## Configuration

Extends the existing `internal/cmd` cobra/pflag setup:

- `--postgres-uri` (or `DATABASE_URL`) — pgx pool DSN.
- `--object-store-dir` — local FS base dir.
- `--host` / `--port` / `--private-key` — existing transport flags.
- Telemetry via `go-faster/sdk` env conventions (OTLP endpoint, etc.).

Migrations (ours + River's) run on startup before the server accepts
connections.

## Observability

As in lilith: `go-faster/sdk` `app.Telemetry` provides OTEL traces, metrics and
structured logs; `otelsql` instruments the pool (`db.Open` mirrors
lilith/internal/db/open.go). Per-request spans wrap dispatcher handling; service
methods annotate spans with `user_id`, TL method, and peer. Key metrics:
active sessions, updates pushed, push failures, `getDifference` size,
object-store bytes in/out, queue depth.

## Single-instance monolith

One process: one PostgreSQL, one local object store, in-process River workers,
in-process session registry and update engine. This is a deliberate constraint:

- `pts` allocation is a row lock — trivially correct without distributed
  coordination.
- The session registry is a local map — no cross-node pub/sub needed.

The **only** thing blocking horizontal scale is the in-memory session registry:
a second instance wouldn't know about sessions on the first. When that day comes,
the seam is a `Notifier` interface behind the registry; a PostgreSQL
`LISTEN/NOTIFY` (or Redis) implementation would broadcast committed updates to
all instances, each pushing to its own local sessions. Nothing else in the design
assumes a single node.

## Testing strategy

- **Unit**: services against `moq` mocks of `DB`/`ObjectStore`/`Queue`.
- **DB integration**: `internal/db` against a real PostgreSQL via
  `testcontainers-go` (as lilith does), exercising migrations + queries.
- **Object store**: FS impl against a temp dir; shared interface conformance
  suite reused for the future S3 impl.
- **End-to-end**: a real gotd/td client (`/src/gotd/td`) drives login, DM
  exchange between two accounts, media round-trip, and reconnect/`getDifference`.
  This is the headline acceptance test per milestone.

## References

- gotd/td low-level primitives we build on: `/src/gotd/td/transport`,
  `exchange` (`ServerExchange.Run`), `crypto`, `proto`/`mt`/`mtproto`, `bin`,
  `tg.ServerDispatcher`.
- gotd/td `tgtest` as the **reference** for the orchestration we reimplement:
  `loop.go`, `exchange.go`, `handle.go`, `send.go`, `conns.go`.
- DB / storage patterns: `/src/ernado/lilith` (`internal/db`, `filestore.go`).
- Protocol reference: `/src/telegram` (tdlib, telegram-bot-api, tdesktop).
