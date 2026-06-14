# Schema (v1: Foundation + DMs)

Status: Draft
Date: 2026-06-11

Proposed PostgreSQL schema for the v1 scope. This is the design target for the
migrations under `internal/db/_migrations`; the authoritative, implemented schema
will live in `internal/db/SCHEMA.md` (kept in sync with migrations, per the
lilith convention). River manages its own tables via its own migrations.

Conventions: `BIGINT` ids, `TIMESTAMPTZ` dates, squirrel dollar placeholders,
upserts via `ON CONFLICT`. Telegram peer ids are positive `BIGINT` for users in
v1 (negative-space encodings for chats/channels are reserved for later).

## auth_keys

MTProto auth keys, backing `AuthKeyStore`. Looked up by `key_id` on every
connection (custom server, no tgtest) so clients reconnect without re-running key
exchange after a restart. See
[architecture](architecture.md#auth-keys-and-sessions).

| Column      | Type        | Constraints           |
|-------------|-------------|-----------------------|
| key_id      | BYTEA       | PRIMARY KEY (8 bytes) |
| auth_key    | BYTEA       | NOT NULL (256 bytes)  |
| dc_id       | INT         | NOT NULL              |
| is_perm     | BOOLEAN     | NOT NULL DEFAULT TRUE |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT now()|
| expires_at  | TIMESTAMPTZ |                       |

## sessions

MTProto session ↔ logged-in user binding.

| Column      | Type        | Constraints                                  |
|-------------|-------------|----------------------------------------------|
| session_id  | BIGINT      | PRIMARY KEY                                  |
| key_id      | BYTEA       | NOT NULL, FK → auth_keys(key_id)             |
| user_id     | BIGINT      | FK → users(id) (null until signed in)        |
| layer       | INT         | NOT NULL DEFAULT 0                            |
| device      | TEXT        | NOT NULL DEFAULT ''                           |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT now()                        |
| last_seen   | TIMESTAMPTZ | NOT NULL DEFAULT now()                        |

## users

| Column      | Type        | Constraints                          |
|-------------|-------------|--------------------------------------|
| id          | BIGSERIAL   | PRIMARY KEY                          |
| access_hash | BIGINT      | NOT NULL                             |
| phone       | TEXT        | UNIQUE                               |
| username    | TEXT        | UNIQUE                               |
| first_name  | TEXT        | NOT NULL DEFAULT ''                  |
| last_name   | TEXT        | NOT NULL DEFAULT ''                  |
| about       | TEXT        | NOT NULL DEFAULT ''                  |
| is_bot      | BOOLEAN     | NOT NULL DEFAULT FALSE               |
| bot_token   | TEXT        | UNIQUE (set for bot accounts)        |
| bot_owner_id | BIGINT     | FK → users(id) (creator, for BotFather bots) |
| photo_file_id | BIGINT    | FK → files(id)                       |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT now()               |

Bots log in with `auth.importBotAuthorization`: the first login with a
well-formed `<id>:<secret>` token auto-provisions a `is_bot` account holding
that `bot_token`, and later logins reuse it.

BotFather (a built-in bot seeded with the fixed id `93372553`) mints tokens
interactively: DMs to it are answered inline by the server, and the `/newbot`
flow creates a `bot_owner_id`-owned bot and returns its token.

## botfather_sessions

A user's position in a multi-step BotFather flow (e.g. `/newbot` asks for a
name, then a username). The row is cleared when the flow completes or is
canceled.

| Column     | Type   | Constraints                                    |
|------------|--------|------------------------------------------------|
| user_id    | BIGINT | PRIMARY KEY, FK → users(id) ON DELETE CASCADE  |
| step       | TEXT   | NOT NULL (e.g. `newbot_name`, `newbot_username`)|
| draft_name | TEXT   | NOT NULL DEFAULT '' (pending bot name)         |

## bot_commands

A bot's published command list, per scope and language. `scope` is the
hex-encoded MTProto `BotCommandScope` so every scope variant (including
peer-specific ones) maps to a distinct row.

| Column      | Type    | Constraints                                      |
|-------------|---------|--------------------------------------------------|
| bot_user_id | BIGINT  | NOT NULL, PK, FK → users(id) ON DELETE CASCADE   |
| scope       | TEXT    | NOT NULL, PK                                     |
| lang_code   | TEXT    | NOT NULL DEFAULT '', PK                          |
| commands    | JSONB   | NOT NULL DEFAULT '[]' ([{command, description}]) |

Managed by `bots.setBotCommands` / `bots.getBotCommands` /
`bots.resetBotCommands`.

## phone_codes

Login code issuance (no real SMS; code handed to a `CodeSink`).

| Column      | Type        | Constraints                          |
|-------------|-------------|--------------------------------------|
| phone       | TEXT        | NOT NULL                             |
| code_hash   | TEXT        | NOT NULL                             |
| code        | TEXT        | NOT NULL                             |
| attempts    | INT         | NOT NULL DEFAULT 0                   |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT now()               |
| expires_at  | TIMESTAMPTZ | NOT NULL                             |

PK `(phone, code_hash)`. Expired rows pruned by a River periodic job.

## contacts

Per-account address book.

| Column          | Type    | Constraints                              |
|-----------------|---------|------------------------------------------|
| owner_user_id   | BIGINT  | NOT NULL, PK, FK → users(id) ON DELETE CASCADE |
| contact_user_id | BIGINT  | NOT NULL, PK, FK → users(id)             |
| first_name      | TEXT    | NOT NULL DEFAULT ''                      |
| last_name       | TEXT    | NOT NULL DEFAULT ''                      |

## dialogs

Per-account view of a conversation.

| Column            | Type        | Constraints                            |
|-------------------|-------------|----------------------------------------|
| owner_user_id     | BIGINT      | NOT NULL, PK, FK → users(id) ON DELETE CASCADE |
| peer_user_id      | BIGINT      | NOT NULL, PK, FK → users(id)           |
| top_message_id    | BIGINT      | NOT NULL DEFAULT 0                      |
| read_inbox_max_id | BIGINT      | NOT NULL DEFAULT 0                      |
| read_outbox_max_id| BIGINT      | NOT NULL DEFAULT 0                      |
| unread_count      | INT         | NOT NULL DEFAULT 0                      |
| pinned            | BOOLEAN     | NOT NULL DEFAULT FALSE                  |
| draft             | TEXT        | NOT NULL DEFAULT ''                     |

## messages

Canonical message content (one row per logical message, shared by both
participants of a DM).

| Column         | Type        | Constraints                          |
|----------------|-------------|--------------------------------------|
| id             | BIGSERIAL   | PRIMARY KEY                          |
| from_user_id   | BIGINT      | NOT NULL, FK → users(id)             |
| peer_user_id   | BIGINT      | NOT NULL, FK → users(id)             |
| date           | TIMESTAMPTZ | NOT NULL DEFAULT now()               |
| text           | TEXT        | NOT NULL DEFAULT ''                  |
| reply_to_msg   | BIGINT      | FK → messages(id)                    |
| media_file_id  | BIGINT      | FK → files(id)                       |
| edit_date      | TIMESTAMPTZ |                                      |
| deleted        | BOOLEAN     | NOT NULL DEFAULT FALSE               |

## message_refs

Per-account view of a message: Telegram message ids are **per-account** local
sequences, so each participant references the canonical message under their own
`message_id`.

| Column        | Type    | Constraints                                  |
|---------------|---------|----------------------------------------------|
| user_id       | BIGINT  | NOT NULL, PK, FK → users(id) ON DELETE CASCADE |
| message_id    | BIGINT  | NOT NULL, PK (per-account local id)          |
| global_id     | BIGINT  | NOT NULL, FK → messages(id) ON DELETE CASCADE |
| out           | BOOLEAN | NOT NULL (true if the user is the sender)    |
| unread        | BOOLEAN | NOT NULL DEFAULT TRUE                        |

Index `message_refs_user_idx` on `(user_id, message_id)`. The per-account
`message_id` is allocated from a per-user counter (next id = current max + 1),
inside the send transaction.

## files

Media metadata. Blob bytes live in the `ObjectStore` under `object_key`.

| Column        | Type        | Constraints                          |
|---------------|-------------|--------------------------------------|
| id            | BIGSERIAL   | PRIMARY KEY                          |
| owner_user_id | BIGINT      | NOT NULL, FK → users(id)             |
| object_key    | TEXT        | NOT NULL (content-addressed key)     |
| size          | BIGINT      | NOT NULL                             |
| mime          | TEXT        | NOT NULL DEFAULT ''                  |
| name          | TEXT        | NOT NULL DEFAULT ''                  |
| sha256        | BYTEA       | NOT NULL                             |
| file_reference| BYTEA       | NOT NULL                             |
| kind          | TEXT        | NOT NULL DEFAULT 'document'          |
| created_at    | TIMESTAMPTZ | NOT NULL DEFAULT now()               |

Index on `sha256` for dedup. `kind` ∈ {photo, document}.

## upload_parts

Crash-safe staging for multipart uploads (alternative to a temp dir).

| Column      | Type    | Constraints                          |
|-------------|---------|--------------------------------------|
| file_id     | BIGINT  | NOT NULL, PK (client-chosen)         |
| part_id     | INT     | NOT NULL, PK                         |
| bytes       | BYTEA   | NOT NULL                             |
| total_parts | INT     | NOT NULL DEFAULT 0                   |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT now()           |

Cleared on finalize; stale rows GC'd by a River periodic job.

## update_state

Current per-account update sequence.

| Column      | Type        | Constraints                          |
|-------------|-------------|--------------------------------------|
| user_id     | BIGINT      | PRIMARY KEY, FK → users(id) ON DELETE CASCADE |
| pts         | BIGINT      | NOT NULL DEFAULT 0                   |
| qts         | BIGINT      | NOT NULL DEFAULT 0  (reserved)       |
| seq         | BIGINT      | NOT NULL DEFAULT 0                   |
| date        | TIMESTAMPTZ | NOT NULL DEFAULT now()               |

`pts` is allocated by `SELECT ... FOR UPDATE` on this row inside the send
transaction (see [architecture](architecture.md#update-sequence)).

## updates_log

Durable per-account update log for `updates.getDifference`.

| Column      | Type        | Constraints                          |
|-------------|-------------|--------------------------------------|
| user_id     | BIGINT      | NOT NULL, PK, FK → users(id) ON DELETE CASCADE |
| pts         | BIGINT      | NOT NULL, PK                         |
| pts_count   | INT         | NOT NULL DEFAULT 1                   |
| type        | TEXT        | NOT NULL (newMessage, editMessage, deleteMessages, readHistory, ...) |
| payload     | JSONB       | NOT NULL (enough to rebuild the tg.Update) |
| date        | TIMESTAMPTZ | NOT NULL DEFAULT now()               |

Index `updates_log_user_pts_idx` on `(user_id, pts)`. Rows below a retention
horizon are pruned by a River periodic job; clients past the horizon get
`updates.differenceTooLong` and resync.

## Notes

- River creates and owns its tables (`river_job`, `river_leader`, ...) via its
  own migration set, run alongside ours at startup.
- Negative peer-id encoding, channels' separate `pts`, secret-chat `qts`, and
  group membership tables are intentionally absent in v1 and added when groups /
  channels land.
