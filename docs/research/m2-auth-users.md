# M2 — Authentication & Users: implementation reference

Research distilled from `/src/gotd/td` (client + dispatcher) and `/src/telegram`.
Citations are `file:line` in `/src/gotd/td`.

## Dispatch model (teled today)
- Handlers register `OnX` closures on `tg.ServerDispatcher`; `Handle(ctx, *bin.Buffer)`
  decodes + routes. Unregistered RPCs hit `Fallback`.
- `mtproto.UnpackInvoke` already strips `InvokeWithLayer` / `InitConnection` /
  `InvokeWithoutUpdates`, so the inner query (e.g. `help.getConfig`) reaches the
  dispatcher directly. `Layer`/device/no-updates are currently discarded (TODO).
- **`Fallback` currently `log.Fatal`s** (`internal/cmd/app.go`) — change it to
  return `tgerr.New(500, "NOT_IMPLEMENTED")` so an unknown RPC can't crash the server.

## RPCs (dispatcher method, request, response) — `tl_server_gen.go`
| # | RPC | OnX | Request | Response |
|---|-----|-----|---------|----------|
| 1 | help.getConfig | `OnHelpGetConfig` (8457) | none | `*tg.Config` |
| 2 | help.getNearestDc | `OnHelpGetNearestDC` | none | `*tg.NearestDC` |
| 3 | auth.sendCode | `OnAuthSendCode` (60) | `*tg.AuthSendCodeRequest` | `tg.AuthSentCodeClass` → `*tg.AuthSentCode` |
| 4 | auth.signIn | `OnAuthSignIn` (94) | `*tg.AuthSignInRequest` | `tg.AuthAuthorizationClass` (`*tg.AuthAuthorization` or `*tg.AuthAuthorizationSignUpRequired`) |
| 5 | auth.signUp | `OnAuthSignUp` (77) | `*tg.AuthSignUpRequest` | `*tg.AuthAuthorization` |
| 6 | auth.logOut | `OnAuthLogOut` (111) | none | `*tg.AuthLoggedOut` |
| 7 | users.getUsers | `OnUsersGetUsers` (2942) | `[]tg.InputUserClass` | `[]tg.UserClass` |
| 8 | users.getFullUser | `OnUsersGetFullUser` (2959) | `tg.InputUserClass` | `*tg.UsersUserFull` |
| 9 | contacts.resolveUsername | `OnContactsResolveUsername` (3247) | `*tg.ContactsResolveUsernameRequest` | `*tg.ContactsResolvedPeer` |
| 10 | contacts.importContacts | `OnContactsImportContacts` (3116) | `[]tg.InputPhoneContact` | `*tg.ContactsImportedContacts` |
| 11 | contacts.getContacts | `OnContactsGetContacts` (3099) | `hash int64` | `tg.ContactsContactsClass` → `*tg.ContactsContacts` |

Methods with a single TL param take it bare (no `*XxxRequest` wrapper): getUsers
(`[]InputUserClass`), getFullUser (`InputUserClass`), getContacts (`hash int64`).

Keep working (client hits during connect): `OnUpdatesGetState`, `OnHelpGetAppConfig`,
`OnHelpGetCountriesList`, `OnHelpGetTermsOfServiceUpdate`.

## Login sequence the gotd client drives
1. MTProto key exchange (done by `internal/mtproto`).
2. First RPC = `InvokeWithLayer{InitConnection{help.getConfig}}` → after unwrap, just
   `help.getConfig`. **Connection not "ready" until this returns a valid `*tg.Config`**
   (client reads `cfg.ThisDC`, needs ≥1 `DCOption` pointing back at us). `conn.go:335`.
3. Unless NoUpdates, client calls `Self()` → `users.getUsers([]{&InputUserSelf{}})`
   (`auth/self.go`). **If session not logged in, return `401 AUTH_KEY_UNREGISTERED`** —
   client treats `auth.IsUnauthorized` as benign and enters auth flow (`connect.go:55`).
   If logged in, return `[]tg.UserClass{self}` with `Self:true`.
4. Auth flow (`auth/flow.go`, `auth/user.go`):
   - `auth.sendCode` (req: PhoneNumber, APIID, APIHash, Settings) → `*tg.AuthSentCode`;
     client needs `PhoneCodeHash` + `Type` (e.g. `&tg.AuthSentCodeTypeSMS{Length:5}`).
   - `auth.signIn` (req: PhoneNumber, PhoneCodeHash, PhoneCode):
     success → `*tg.AuthAuthorization{User: self}`; no account →
     `*tg.AuthAuthorizationSignUpRequired{}` (a **union member, not an error**).
   - `auth.signUp` (req: PhoneNumber, PhoneCodeHash, FirstName, LastName) →
     `*tg.AuthAuthorization{User: newSelf}`.

Minimal login set: help.getConfig, auth.sendCode, auth.signIn (+signUp), users.getUsers
(401 pre-login, self post-login).

## Errors
- Return `*tgerr.Error` via `tgerr.New(code, "MSG")`. `internal/mtproto` already converts
  it to wire `mt.RPCError` (handle.go catch + send.go `SendErr`). Just `return nil, tgerr.New(...)`.
- `auth.IsUnauthorized` = `tgerr.IsCode(err, 401)`. Pre-login getUsers → `401 AUTH_KEY_UNREGISTERED`.
- Do NOT return `406 SESSION_PASSWORD_NEEDED` (2FA out of scope).
- signIn errors: `400 PHONE_CODE_INVALID|PHONE_CODE_EXPIRED|PHONE_CODE_EMPTY|PHONE_NUMBER_INVALID|PHONE_NUMBER_UNOCCUPIED`, `420 FLOOD_WAIT_X`.
- sendCode: `400 PHONE_NUMBER_INVALID|API_ID_INVALID`, `406 PHONE_NUMBER_BANNED`.
- signUp: `400 PHONE_NUMBER_OCCUPIED|FIRSTNAME_INVALID|PHONE_CODE_INVALID`.
- Fake auth for M2: store `(phone, phone_code_hash, code)` on sendCode; validate on signIn;
  return `PHONE_CODE_INVALID` only on mismatch.

## tg.User / tg.UserFull for self
- **Never set `Flags` by hand** — `Encode` recomputes from set fields / `SetX` helpers.
- `tg.User` self: `ID int64` (= peer id), `Self:true`, `AccessHash int64`, `FirstName`,
  `LastName`, `Username`, `Phone`, `Photo=&tg.UserProfilePhotoEmpty{}`,
  `Status=&tg.UserStatusEmpty{}` (or Online).
- `access_hash`: self uses `InputUserSelf` (no hash needed). For other users, client echoes
  the `access_hash` you issued back in `tg.InputUser{UserID, AccessHash}` — generate a stable
  value (e.g. deterministic), persist, validate. Must be consistent across responses.
- `tg.UsersUserFull{FullUser, Chats, Users}` — **must populate `Users`** with the brief
  `*tg.User` for the same id. `UserFull`: `ID`, `About`, `Settings tg.PeerSettings{}` (value),
  `NotifySettings tg.PeerNotifySettings{}` (value), `ProfilePhoto` optional, `CommonChatsCount`.

## Gotchas
- `users.getUsers` is both self-detection and the unauthorized gate — wrong here breaks the flow.
- `InputUserClass` switch: `*InputUserSelf` → session user; `*InputUser{UserID,AccessHash}` →
  validate+lookup; unresolved entry in getUsers → `*tg.UserEmpty{ID}` (keep slice indices).
- `AuthAuthorizationSignUpRequired` triggers signup; not an error.
- `contacts.resolveUsername` (DMs): return `Peer:&tg.PeerUser{UserID}` + `*tg.User` in `Users`
  (current stub returns a channel — change it). Miss → `400 USERNAME_NOT_OCCUPIED`.
- `contacts.importContacts` → `*tg.ContactsImportedContacts{Imported:[]ImportedContact{UserID,ClientID}, Users, RetryContacts}`; client matches ClientID→UserID.
- `contacts.getContacts(hash)` → `*tg.ContactsContacts{Contacts:[]Contact{UserID,Mutual}, SavedCount, Users}` or `*tg.ContactsContactsNotModified{}` if hash matches.
- `UpdateUserName{UserID, Usernames, FirstName, LastName}` to push profile changes (post-login).
- tgtest `services/` use the low-level `tgtest.Dispatcher` (TypeID switch), NOT `ServerDispatcher.OnX`;
  semantic reference only. **No auth/users service ships in tgtest — build from scratch.**

## Key references
- Dispatcher: `tg/tl_server_gen.go`. Client auth: `telegram/auth/{flow,user,self,signup}.go`.
  Connect: `telegram/connect.go:53`, `telegram/internal/manager/conn.go:335`. Errors: `tgerr/error.go`.
  Types: `tl_user_gen.go`, `tl_user_full_gen.go`, `tl_users_user_full_gen.go`, `tl_contacts_*_gen.go`.
