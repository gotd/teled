# M4 — Updates engine: implementation reference

Citations in `/src/gotd/td`. The client manager we must satisfy lives in
`telegram/updates/`; the server emit pattern is `tgtest/services/messages/send.go`.

## RPCs
- `OnUpdatesGetState` (`tl_server_gen.go:8177`) — empty request → `*tg.UpdatesState`.
- `OnUpdatesGetDifference` (8194) — `*tg.UpdatesGetDifferenceRequest` → `tg.UpdatesDifferenceClass`.
- Live push via the `internal/mtproto` server's `SendUpdates(ctx, Session, ...UpdateClass)`
  (wraps in `&tg.Updates{Updates, Date}`, sends as `proto.MessageFromServer`).

## pts / pts_count / seq / date (DMs, v1)
- **pts**: per-account monotonic counter for the common message box. Per recipient account —
  each side of a conversation has its own pts stream. v1 = single common pts per account.
- **pts_count**: units one update consumes. Client applies only if `localPts + pts_count == update.Pts`
  (`gap_check.go:18`). New/edit/read = 1; `UpdateDeleteMessages` = `len(messages)`.
- pts-bearing updates (`tl_updates_classifier_gen.go:34-57`): UpdateNewMessage, DeleteMessages,
  ReadHistoryInbox, ReadHistoryOutbox, WebPage, ReadMessagesContents, EditMessage, FolderPeers,
  PinnedMessages. qts updates = secret/bot → out of scope, keep qts=0.
- **seq**: separate per-account sequence for the `Updates`/`UpdatesCombined` wrappers (counts the
  wrapper, not inner updates). Carried only on those wrappers; `UpdateShort` has none.
- **date**: server unix time, monotonic non-decreasing.

## updates.getState
Return `*tg.UpdatesState{Pts, Qts(0), Date, Seq, UnreadCount}` from the persisted per-account state.
Client calls it on bootstrap with no stored state and adopts it as baseline — must be the TRUE
current pts/seq or the client immediately sees a gap.

## updates.getDifference
Request carries client's last-acked `Pts, Qts, Date` (+optional PtsLimit). Return all events with
pts > request.Pts as one of:
- **`UpdatesDifferenceEmpty{Date, Seq}`** — nothing newer. **Steady-state reconnect answer.**
- **`UpdatesDifference{NewMessages []MessageClass, NewEncryptedMessages(empty), OtherUpdates []UpdateClass, Chats, Users, State UpdatesState}`** — full backlog; client adopts `State` and stops.
  NewMessages are full message objects (NOT wrapped); client converts them to UpdateNewMessage.
- **`UpdatesDifferenceSlice{..., IntermediateState UpdatesState}`** — partial; client applies, sets
  state to IntermediateState, calls getDifference again. **IntermediateState.Pts must strictly advance.**
- **`UpdatesDifferenceTooLong{Pts}`** — pts too far behind / history trimmed; client resets pts, drops
  cached state, re-requests. Use sparingly.
- Decision: `req.Pts==current` → Empty; full backlog fits → Difference; batching → Slice (advancing);
  below retained floor → TooLong{current}.

## CRITICAL: client manager rules (avoid infinite getDifference) — `telegram/updates/`
1. **Contiguous pts**: every pushed pts-bearing update must have `Pts = prevPts + PtsCount`, no holes,
   no overlaps. A single skipped/reused pts → 500ms gap timer → getDifference.
2. **pts_count must match reality** (delete = len). Wrong count permanently desyncs localPts.
3. **getState/getDifference State must agree with the live stream**: the pts you report becomes the
   client's localState; the next push must satisfy rule 1 against it.
4. **seq=0 disables seq gap checks** (`gap_check.go:14`, `state.go:299`). **Policy: push `tg.Updates`
   with `Seq=0`** (exactly what tgtest does) and rely only on per-pts ordering. Only use nonzero seq
   if you maintain a correct contiguous per-account seq.
5. **getDifference must terminate**: Slice/TooLong recurse; never return State.Pts ≤ req.Pts.
6. Client also refetches on 15min idle + 500ms gap timer; self-heals only if getDifference is consistent.
7. **Unknown peers force getDifference**: always include referenced `Users` (and Chats) in the
   pushed `tg.Updates` wrapper, else every new-sender message triggers a difference fetch.

## DM update types + wrappers
| Event | Update | Pts | PtsCount |
|---|---|---|---|
| new | `UpdateNewMessage{Message, Pts, PtsCount}` | next | 1 |
| edit | `UpdateEditMessage{Message, Pts, PtsCount}` | next | 1 |
| delete | `UpdateDeleteMessages{Messages []int, Pts, PtsCount}` | next | len |
| read inbox | `UpdateReadHistoryInbox{Peer, MaxID, StillUnreadCount, Pts, 1}` | next | 1 |
| read outbox | `UpdateReadHistoryOutbox{Peer, MaxID, Pts, 1}` | next | 1 |

Each consumes pts from the **recipient's** stream. `UpdateMessageID{ID, RandomID}` (no pts) returned
on the send RPC result, not pushed.

Wrappers: `UpdateShort{Update, Date}` (single, no seq); **`Updates{Updates, Users, Chats, Date, Seq=0}`
— recommended live push, include Users/Chats**; `UpdatesCombined{...SeqStart}` only for contiguous seq
ranges; `UpdatesTooLong` to force a getDifference.

## Gotchas
- seq=0 everywhere in v1; if ever nonzero it must be strictly contiguous per account.
- date monotonic; use one server clock.
- pts is per-account, not per-pair: A→B advances both A's and B's pts independently (allocate two).
- Serialize pts allocation + persist + push per account (mutex / row lock) so the client never sees
  pts go backward across a live-push vs getDifference race.
- DifferenceSlice IntermediateState.Pts must be > req.Pts or the client loops forever.

## Refs
`tgtest/services/messages/send.go:21-89`; `tgtest/send.go:162-176`; `telegram/updates/storage.go`,
`gap_check.go`, `sequence_box.go`, `state.go:237-543`; `tg/tl_updates_classifier_gen.go:34-119`.
Note: tgtest messages service only pushes live UpdateNewMessage; M4 adds persisted pts + getState/getDifference.
