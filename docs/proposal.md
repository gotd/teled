# Proposal: Full-featured Telegram server

Status: Draft
Author: teled maintainers
Date: 2026-06-11

## Summary

`teled` today is a stub MTProto server: it wires gotd/td's `tgtest.Server`
(transport, auth-key exchange, MTProto encryption, in-memory sessions) to a
`tg.ServerDispatcher` whose handlers return hardcoded responses. It is enough to
make a real client complete key exchange and a fake login, but it stores nothing
and implements no real behaviour.

This proposal turns `teled` into a real, stateful Telegram server backed by
PostgreSQL, with media stored through an S3-compatible object-storage
abstraction (local filesystem first), background work on a PostgreSQL-backed
queue, and a real update-delivery engine. It is scoped as a **single-instance
monolith**.

## Goals

- Persist all account, contact, dialog, message and media state in PostgreSQL.
- Implement a real authentication flow (`auth.sendCode` / `auth.signIn`) and
  per-account state.
- Implement private 1:1 chats (DMs): send/edit/delete messages, read state,
  dialogs, history.
- Implement media upload/download (`upload.saveFilePart` / `upload.getFile`)
  with blobs stored behind an object-storage interface.
- Implement a real update-delivery engine: live push over the active connection
  plus `updates.getDifference` reconciliation, backed by a persisted per-account
  update sequence (`pts`).
- Run background work (media processing, retries, periodic cleanup) on a
  PostgreSQL-backed queue.
- Keep the design observable (OpenTelemetry traces/metrics/logs) and testable.

## Non-goals (v1)

- Groups, supergroups, channels and broadcast fan-out. The schema and update
  engine are designed not to preclude them, but they are a later milestone.
- Secret chats (`qts` / end-to-end encryption), calls (tgcalls), payments,
  stories, bots/inline, stickers beyond stubs.
- Horizontal scaling / multi-instance. Explicitly single-instance (see
  [architecture](architecture.md#single-instance-monolith)).
- Real SMS delivery. Login codes are issued through a pluggable code sink
  (logged / fixed in dev).

## Scope decision: "Foundation + DMs"

v1 targets the smallest coherent end-to-end slice that exercises every layer:

| Domain      | v1 surface                                                       |
|-------------|------------------------------------------------------------------|
| Transport   | gotd/td `tgtest.Server` (single DC), key exchange, sessions      |
| Auth        | `auth.sendCode`, `auth.signIn`, `auth.signUp`, `auth.logOut`     |
| Users       | self, `users.getUsers`, `users.getFullUser`, profile fields      |
| Contacts    | `contacts.importContacts`, `resolveUsername`, `getContacts`      |
| Dialogs     | `messages.getDialogs`, pinned, read state, drafts                |
| Messages    | `sendMessage`, `editMessage`, `deleteMessages`, `getHistory`, `readHistory`, `sendMedia` |
| Media       | `upload.saveFilePart`/`saveBigFilePart`, `upload.getFile`, photos & documents |
| Updates     | live push + `updates.getState` / `updates.getDifference`         |
| Help/config | `help.getConfig`, `getAppConfig`, `getCountriesList` (real-ish)  |

## Key technology decisions

These mirror the `lilith` reference stack unless noted; deviations are called
out in the [architecture](architecture.md) document.

| Concern            | Choice                                   | Rationale |
|--------------------|------------------------------------------|-----------|
| Database           | PostgreSQL via `jackc/pgx/v5` + `pgxpool`| Same as lilith; `stdlib.OpenDBFromPool` for `database/sql` interop. |
| Query building     | `Masterminds/squirrel` (dollar)          | Same as lilith. |
| Migrations         | `golang-migrate/migrate/v4`, embedded    | Same as lilith; `_migrations/*.sql` via `embed.FS`. |
| Background queue   | **River** (`riverqueue/river`)           | PostgreSQL-backed (`FOR UPDATE SKIP LOCKED`), typed jobs, retries, periodic jobs. Decided over hand-rolled LISTEN/NOTIFY and the `pgmq` extension. |
| Object storage     | S3-shaped interface, **local FS** impl   | Matches stated focus; S3 backend deferred but interface-compatible. |
| Update delivery    | **Push + `getDifference`**               | Full Telegram semantics: live push over the active conn plus difference reconciliation on reconnect. |
| MTProto layer      | **Custom server** built on gotd/td low-level primitives (`transport`, `exchange`, `crypto`, `proto`/`mt`/`mtproto`) + `tg.ServerDispatcher` | We drop `tgtest.Server` and own the connection loop, so auth-key/session lookup, persistence and update push are first-class rather than fighting tgtest's in-memory internals. |
| Errors / logging   | `go-faster/errors`, `go-faster/sdk` (`zctx`, `app.Telemetry`) | Same as lilith. |
| Observability      | OpenTelemetry via `go-faster/sdk`, `otelsql` | Same as lilith. |

## Custom MTProto server (dropping tgtest)

We do **not** use `tgtest.Server`. It is a ~700-line in-memory orchestration
layer whose `users` map (`tgtest/conns.go`) stores auth keys and connections
with no storage seam: an unknown `auth_key_id` is rejected outright, so a client
cannot reconnect with a persisted key after a restart.

Instead we reimplement just that orchestration in `internal/mtproto`, composing
the same gotd/td low-level packages tgtest itself uses (`transport`, `exchange`,
`crypto`, `proto`/`mt`/`mtproto`, `bin`). The cryptography — server-side DH key
exchange, MTProto v2 message encryption, transport framing/obfuscation — is
reused unchanged; only the connection lifecycle is ours.

The immediate payoff: in our connection loop the `auth_key_id` lookup goes
through `AuthKeyStore` (PostgreSQL), so **auth keys and sessions persist across
restarts by design** — the previous "known gap" is resolved. The cost is that we
now own MTProto service-message correctness (acks, `bad_msg_notification`,
`msg_id`/`seq_no` validation, salt rotation, containers, gzip); `tgtest/handle.go`
and the client-side `mtproto/handle_*.go` are the reference implementations, and
conformance is verified against a real gotd/td client. See
[architecture](architecture.md#transport--mtproto-layer-internalmtproto).

## Milestones

1. **M1 — Foundation + custom server**: project skeleton (packages below),
   PostgreSQL wiring, migrations, telemetry, object-store interface + FS impl,
   River setup, and the `internal/mtproto` server — accept loop, server-side key
   exchange, `auth_key_id` lookup via `AuthKeyStore`, MTProto message
   handling, and dispatcher wiring. This replaces the current `tgtest.Server`
   bootstrap in `internal/cmd`; the existing stub handlers are re-hosted on the
   new server so it stays bootable and a gotd/td client can still complete key
   exchange and the fake login.
2. **M2 — Auth & users**: real `auth.*` flow, `users`/`contacts` persisted,
   session ↔ user binding.
3. **M3 — Messaging**: DMs send/edit/delete/history/read, dialogs, per-account
   message-id space, update sequence (`pts`) writes.
4. **M4 — Updates engine**: session registry, live push, `updates.getState` /
   `getDifference`.
5. **M5 — Media**: multipart upload staging, object-store persistence, download
   ranges, `sendMedia`, file references; media jobs on River.
6. **M6 — Hardening**: conformance against real gotd/td client, load smoke test,
   docs, metrics dashboards.

## Validation

The primary correctness oracle is a real gotd/td **client** (`/src/gotd/td`)
talking to `teled`: it must complete login, send/receive DMs across two
accounts, upload/download a photo, disconnect and reconcile via
`getDifference`. Reference behaviour is cross-checked against the official
sources under `/src/telegram` (tdlib, telegram-bot-api).

## Open questions

- Whether to start `internal/mtproto` greenfield (composing gotd/td primitives,
  with `tgtest` only as a reference) or by vendoring/forking `tgtest` and editing
  it down. Default: greenfield, no dead code.
- Whether per-account `pts` is allocated via a PostgreSQL sequence per user or a
  row-locked counter (see [architecture](architecture.md#update-sequence)).
