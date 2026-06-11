# M3 — Messaging (DMs): implementation reference

Citations are `file:line` in `/src/gotd/td`. **Best reference:
`tgtest/services/messages/`** — adapt it, swapping in-memory maps for Postgres.

## RPCs — `tg/tl_server_gen.go`
| # | Method | OnX | Request | Response |
|---|--------|-----|---------|----------|
| 1 | messages.sendMessage | `OnMessagesSendMessage` (3734) | `*MessagesSendMessageRequest` | `UpdatesClass` |
| 2 | messages.editMessage | `OnMessagesEditMessage` (4573) | `*MessagesEditMessageRequest` | `UpdatesClass` |
| 3 | messages.deleteMessages | `OnMessagesDeleteMessages` (3679) | `*MessagesDeleteMessagesRequest` | `*MessagesAffectedMessages` |
| 4 | messages.getHistory | `OnMessagesGetHistory` (3611) | `*MessagesGetHistoryRequest` | `MessagesMessagesClass` |
| 5 | messages.readHistory | `OnMessagesReadHistory` (3645) | `*MessagesReadHistoryRequest` | `*MessagesAffectedMessages` |
| 6 | messages.getDialogs | `OnMessagesGetDialogs` (3594) | `*MessagesGetDialogsRequest` | `MessagesDialogsClass` |
| 7 | messages.toggleDialogPin | `OnMessagesToggleDialogPin` (4958) | `*...Request` | `bool` |
| 8 | messages.reorderPinnedDialogs | `OnMessagesReorderPinnedDialogs` (4979) | `*...Request` | `bool` |
| 9 | messages.getPinnedDialogs | `OnMessagesGetPinnedDialogs` (5000) | `folderid int` | `*MessagesPeerDialogs` |
| 10 | messages.saveDraft | `OnMessagesSaveDraft` (4666) | `*...Request` | `bool` |
| 11 | messages.getAllDrafts | `OnMessagesGetAllDrafts` (4687) | none | `UpdatesClass` |

Build order: 1 → 4 → 6 → 5 → 2 → 3 → drafts → pinned.

## tgtest/services/messages flow (mirror this)
- State: `users` registry; `self map[authKeyID]userID`; `sessions map[userID][]Session`
  (push targets); `history map[{self,peer}][]*tg.Message` (**each side stores its OWN copy**);
  `lastMsgID`, `lastPts` global counters → **make per-account in teled**.
- `nextMessage()` = `lastMsgID++; lastPts++` under mutex; called once per copy (sender+recipient).
- **sendMessage** (`send.go:21-89`):
  1. resolve self (session) + peer (`InputPeerClass`).
  2. `date = now`. Allocate `(msgID, pts)`, build sender copy
     `&tg.Message{ID, Out:true, FromID:&PeerUser{self}, PeerID:&PeerUser{peer}, Date, Message}`,
     `SetFlags()`, store under `(self,peer)`.
  3. if `peer != self`: allocate `(inMsgID, incomingPts)`, build recipient copy
     `&tg.Message{ID:inMsgID, FromID:&PeerUser{self}, PeerID:&PeerUser{self}, Date, Message}`
     (no Out; recipient's PeerID = the other party = sender), store under `(peer,self)`.
  4. push to recipient sessions (best-effort, never fail sender):
     `SendUpdates(ctx, session, &tg.UpdateNewMessage{Message:incoming, Pts:incomingPts, PtsCount:1})`.
  5. return `&tg.Updates{Updates:[]{&UpdateMessageID{ID:msgID, RandomID:req.RandomID}, &UpdateNewMessage{Message:out, Pts, PtsCount:1}}, Users:[self,peer], Date}`.
- **getHistory** (`history.go`): read `(self,peer)`, reverse newest-first, cap limit, return
  `&tg.MessagesMessages{Messages, Users:[self,peer]}`. Add `offset_id`/`add_offset`/`Count`+`Slice`.
- Test asserts (`messages_test.go:76`): result is `*tg.Updates` with **UpdateMessageID first**,
  `mid.RandomID==req.RandomID`, `mid.ID==sentMessage.ID`, UpdateNewMessage message `Out=true`.

## Updates emitted per event
| Event | Return (to caller) | Push (to other party / other sessions) |
|---|---|---|
| sendMessage | `*tg.Updates{UpdateMessageID, UpdateNewMessage(Out), Users, Date}` | recipient: `UpdateNewMessage{recipientCopy, incomingPts, 1}` |
| editMessage | `*tg.Updates{UpdateEditMessage{Message, Pts, 1}, Users, Date}` | peer: `UpdateEditMessage` recipient copy; bump `EditDate` |
| deleteMessages | `*tg.MessagesAffectedMessages{Pts, PtsCount=len(deleted)}` | `UpdateDeleteMessages{Messages:[]int (each side's local ids), Pts, PtsCount}` |
| readHistory | `*tg.MessagesAffectedMessages{Pts, PtsCount}` | reader sessions: `UpdateReadHistoryInbox{Peer, MaxID, StillUnreadCount, Pts, 1}`; peer: `UpdateReadHistoryOutbox{Peer:reader, MaxID, Pts, 1}` |

- `UpdateMessageID{ID, RandomID}` (no pts) maps client random_id → server id; **must be first**.
- pts is **per-account**, monotonic, `+pts_count` per event (1 for new/edit/read; len for delete).
  Sender and recipient each get their own pts increment for the same logical message.
- Compact alternatives exist (`UpdateShortSentMessage`, `UpdateShortMessage`) but prefer the
  full `Updates` form (carries random_id + users).

## Per-account message-id model (confirmed correct)
DM message ids are **per-account local sequences**; same logical message has different ids per side.
- `messages` (canonical, `global_id` PK): `from_user_id`, `peer_user_id`, `text`, `entities`,
  `date`, `edit_date`, `reply_to_global_id`, `deleted`, `random_id`.
- `message_refs(user_id, message_id, global_id)`: one row per account-copy; `message_id` = per-user
  local id; PK `(user_id, message_id)`; unique `(user_id, global_id)`.
- Per-user local-id counter + per-user pts counter, allocated atomically.
- Reconstruct `tg.Message` from ref+canonical: `ID=ref.message_id`; `Out=(from_user_id==ref.user_id)`;
  `FromID=PeerUser{from_user_id}`; `PeerID=PeerUser{other party from ref.user_id POV}`; `SetFlags()`.
  Translate `reply_to` global→local via message_refs.
- getDialogs per account: `top_message`=max local id; `read_inbox_max_id`/`read_outbox_max_id` from
  stored read pointers; `unread_count`=incoming with local id > read_inbox_max_id.

## Minimal types + gotchas
- `tg.Message` minimal: `ID, Out(sender only), FromID, PeerID, Date, Message` (+Entities/ReplyTo/EditDate).
  **Always `SetFlags()`** before returning any flagged struct (Message, Dialog).
- `tg.Dialog`: `Peer, TopMessage, ReadInboxMaxID, ReadOutboxMaxID, UnreadCount, NotifySettings{}`;
  `Pinned`+`SetFlags`; drafts via `SetDraft`.
- `tg.messages.Messages{Messages, Users, Chats}`; use `MessagesMessagesSlice{Count, Inexact}` for
  pagination. **Users must include every referenced user.**
- `tg.messages.Dialogs{Dialogs, Messages(top per dialog), Users, Chats}`; `MessagesDialogsSlice{Count}`.
  `getPinnedDialogs` → `*tg.MessagesPeerDialogs{Dialogs, Messages, Users, Chats, State}` (fill State pts/qts/date/seq).
- `tg.messages.AffectedMessages{Pts, PtsCount}` (delete + readHistory).
- Self-chat (`peer==self`): skip 2nd copy/delivery. Delivery best-effort. History newest-first.

## Refs
`tgtest/services/messages/{send,history,messages,identity,messages_test}.go`;
`tg/tl_server_gen.go`; `telegram/internal/upconv/upconv.go`; `telegram/message/unpack/message.go`.
