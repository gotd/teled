# M5 — Media (upload/download): implementation reference

Citations in `/src/gotd/td`. **Best reference: `tgtest/services/file/`** — working
server-side impl of all four RPCs; mirror into `ServerDispatcher.OnX` handlers.

## RPCs — `tg/tl_server_gen.go`
| # | OnX (line) | Request | Response |
|---|---|---|---|
| 1 | `OnUploadSaveFilePart` (8313) | `*tg.UploadSaveFilePartRequest` | `bool` |
| 2 | `OnUploadSaveBigFilePart` (8351) | `*tg.UploadSaveBigFilePartRequest` | `bool` |
| 3 | `OnMessagesSendMedia` (3751) | `*tg.MessagesSendMediaRequest` | `tg.UpdatesClass` |
| 4 | `OnUploadGetFile` (8334) | `*tg.UploadGetFileRequest` | `tg.UploadFileClass` → `*tg.UploadFile` |

`bool` auto-wrapped to BoolTrue/False; return `false` to make client retry the part.
Optional: `OnUploadGetFileHashes` (only with WithVerify), `OnMessagesUploadMedia` (bots).

## Upload
Constants (`constant/uploads.go`): small/big threshold `UploadMaxSmallSize=10MB`;
`UploadMaxParts=3999`; padding 1KB; `UploadMaxPartSize=512KB`.
Client (`telegram/uploader/`): default part size **128KB** (may auto-grow, divisor of 512KB) — **infer
part size from the first part, don't hardcode**. big when total>10MB or unknown size. Parts numbered
from 0. Small uploads carry MD5 in `InputFile.MD5Checksum`; big (`InputFileBig`) none.

Requests: `UploadSaveFilePartRequest{FileID int64, FilePart int, Bytes}`;
`UploadSaveBigFilePartRequest{FileID, FilePart, FileTotalParts, Bytes}`.
`tg.InputFile{ID, Parts, Name, MD5Checksum}`; `tg.InputFileBig{ID, Parts, Name}`.

Staging (mirror `file/upload.go:58`): write each part at `offset=partSize*FilePart` (WriteAt/sparse),
keyed by **`(user/session, FileID)`** (FileID is client-random, not globally unique). Validate
(`upload.go:35`): empty→`FILE_PART_EMPTY`, >512KB→`FILE_PART_TOO_BIG`, changed size→`FILE_PART_SIZE_CHANGED`,
bad align→`FILE_PART_SIZE_INVALID`, part∉[0,3999]→`FILE_PART_INVALID`. Store part size from first part.
Assemble into ObjectStore only at sendMedia time.

## messages.sendMedia
`MessagesSendMediaRequest{Peer, Media InputMediaClass, Message, RandomID}`. **RandomID = dedup key —
reuse the M3 send path (idempotency + Updates emission).**
Input media: `InputMediaUploadedPhoto{File InputFileClass, ...}`; `InputMediaUploadedDocument{File, Thumb?, MimeType, Attributes (filename), ...}`. (Resend variants `InputMediaPhoto/Document` later.)
Handling:
1. `Media.File.GetID()` → locate staged parts → assemble → `ObjectStore.Put`.
2. Photo: `tg.Photo{ID, AccessHash, FileReference, Date, DCID=<our DC>, Sizes:[]PhotoSizeClass{&tg.PhotoSize{Type:"x", W, H, Size}}}`. **≥1 real PhotoSize mandatory.**
   Document: `tg.Document{ID, AccessHash, FileReference, Date, MimeType, Size, DCID, Attributes}`.
3. Wrap: `tg.MessageMediaPhoto` (`SetPhoto`) / `tg.MessageMediaDocument` (`SetDocument`).
4. Persist message with media; **emit M3-style `tg.UpdatesClass`** for both DM participants
   (the `tg.Message.Media` = the MessageMediaPhoto/Document). Same Updates shape as text.

## Download — getFile
Client builds the location (`telegram/query/messages/specialize.go:120`): Photo → pick largest sized
size, `tg.InputPhotoFileLocation{ID, AccessHash, FileReference, ThumbSize:<size.Type>}`; Document →
`AsInputDocumentFileLocation()` = `{ID, AccessHash, FileReference}` (empty ThumbSize = full file).
**If no sized PhotoSize exists, download is impossible.**

`UploadGetFileRequest{Precise, CDNSupported, Location InputFileLocationClass, Offset int64, Limit int}`.
Map Location→blob (ID[+ThumbSize] selects which bytes), validate AccessHash + FileReference, then
`ObjectStore.GetRange(Offset, Limit)`. Return `*tg.UploadFile{Type: &tg.StorageFilePartial{}, Mtime, Bytes:data[:n]}`.

Alignment (`telegram/downloader/`): client part size 512KB; offset advances by partSize each call →
**every request already aligned**. **EOF = a chunk with `len < partSize`** → return exactly Limit for
full chunks, short read only at true EOF. Don't return more than Limit. (Mirror `file/download.go:40`.)

## file_reference
Opaque server-chosen `[]byte`; client echoes it verbatim in the location. Issue at Photo/Document
creation (sendMedia). v1: non-expiring, no regeneration; just be consistent so a reference from
sendMedia is accepted by later getFile. Validate it resolves to the same `(ID, AccessHash)`. tgtest
skips validation (`download.go:26`). **Three distinct ids: `file_id` (client, InputFile) ≠ media id
(server, Photo/Document) ≠ file_reference.**

## Minimal viable (upload photo → DM → download)
1. `OnUploadSaveFilePart`: stage at `partSize*FilePart` keyed by `(user, FileID)`; record part size; `true`.
2. `OnMessagesSendMedia` with `*InputMediaUploadedPhoto`: assemble → Put; build Photo (≥1 PhotoSize,
   DCID=our DC, non-empty FileReference); wrap MessageMediaPhoto; persist; emit M3 Updates with RandomID.
3. `OnUploadGetFile`: resolve `InputPhotoFileLocation{ID,ThumbSize}` → GetRange → `*tg.UploadFile{StorageFilePartial, Bytes}`.

## Gotchas
- **DCID must equal our server DC** in Photo/Document, else client routes getFile to a missing DC.
- ≥1 `sizedPhoto` size (PhotoSize/PhotoCachedSize/PhotoSizeProgressive) with W,H>0 — `PhotoStrippedSize`/`PhotoSizeEmpty` don't count.
- Part size not fixed (128KB default, may grow) — infer from first part.
- Don't over-return on getFile; short read = EOF.
- `access_hash` round-trips (Photo.AccessHash → InputPhotoFileLocation.AccessHash); validate.
- Big-file path only >10MB/unknown; staging identical (no MD5).

## Refs
`tgtest/services/file/{upload,download,file}.go`; `constant/uploads.go`; `telegram/uploader/{part,small,big,uploader}.go`;
`telegram/downloader/{downloader,reader,builder}.go`; `telegram/query/messages/specialize.go`;
types `tl_input_file_gen.go`, `tl_input_media_gen.go`, `tl_photo_gen.go`, `tl_photo_size_gen.go`,
`tl_document_gen.go`, `tl_input_file_location_gen.go`, `tl_upload_file_gen.go`, `tl_message_media_gen.go`.
