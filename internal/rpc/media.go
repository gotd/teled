package rpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
)

const maxPartSize = 512 * 1024

func (h *Handler) requireStore() error {
	if h.store == nil {
		return tgerr.New(500, "NOT_IMPLEMENTED")
	}
	return nil
}

// uploadSaveFilePart stages a small-file upload part.
func (h *Handler) uploadSaveFilePart(ctx context.Context, req *tg.UploadSaveFilePartRequest) (bool, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return false, err
	}
	if len(req.Bytes) == 0 || len(req.Bytes) > maxPartSize {
		return false, tgerr.New(400, "FILE_PART_INVALID")
	}
	h.staging.file(stagingKey{caller.ID, req.FileID}).put(req.FilePart, req.Bytes)
	return true, nil
}

// uploadSaveBigFilePart stages a big-file upload part.
func (h *Handler) uploadSaveBigFilePart(ctx context.Context, req *tg.UploadSaveBigFilePartRequest) (bool, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return false, err
	}
	if len(req.Bytes) == 0 || len(req.Bytes) > maxPartSize {
		return false, tgerr.New(400, "FILE_PART_INVALID")
	}
	h.staging.file(stagingKey{caller.ID, req.FileID}).put(req.FilePart, req.Bytes)
	return true, nil
}

// messagesSendMedia assembles a staged photo upload, stores it, and sends it as
// a DM. The media is returned in the response updates.
func (h *Handler) messagesSendMedia(ctx context.Context, req *tg.MessagesSendMediaRequest) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.requireStore(); err != nil {
		return nil, err
	}
	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return nil, err
	}

	fileID, ok := uploadedFileID(req.Media)
	if !ok {
		return nil, tgerr.New(400, "MEDIA_INVALID")
	}
	staged, ok := h.staging.take(stagingKey{caller.ID, fileID})
	if !ok {
		return nil, tgerr.New(400, "FILE_REFERENCE_EMPTY")
	}

	data := staged.assemble()
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:])
	if err := h.store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), teled.PutOptions{}); err != nil {
		return nil, h.internal(ctx, "store put", err)
	}

	file, err := h.db.SaveFile(ctx, teled.File{
		OwnerUserID: caller.ID, ObjectKey: key, Size: int64(len(data)), SHA256: sum[:], Kind: "photo",
	})
	if err != nil {
		return nil, h.internal(ctx, "save file", err)
	}

	sent, err := h.db.SendMessage(ctx, caller.ID, peer.ID, req.Message, req.RandomID, file.ID)
	if err != nil {
		return nil, h.internal(ctx, "send media message", err)
	}

	out := dmMessage(teled.Message{
		LocalID: sent.SenderLocalID, FromUserID: caller.ID, PeerUserID: peer.ID,
		Out: true, Text: req.Message, Date: sent.Date, Media: &file,
	})

	if !sent.SelfChat {
		incoming := dmMessage(teled.Message{
			LocalID: sent.RecipientLocalID, FromUserID: caller.ID, PeerUserID: caller.ID,
			Out: false, Text: req.Message, Date: sent.Date, Media: &file,
		})
		h.push(ctx, peer.ID,
			[]tg.UserClass{toTGUser(caller, false), toTGUser(peer, true)},
			int(sent.Date.Unix()),
			&tg.UpdateNewMessage{Message: incoming, Pts: sent.RecipientPts, PtsCount: 1},
		)
	}

	return &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: int(sent.SenderLocalID), RandomID: req.RandomID},
			&tg.UpdateNewMessage{Message: out, Pts: sent.SenderPts, PtsCount: 1},
		},
		Users: []tg.UserClass{toTGUser(caller, true), toTGUser(peer, false)},
		Date:  int(sent.Date.Unix()),
	}, nil
}

// uploadGetFile serves a ranged read of stored media.
func (h *Handler) uploadGetFile(ctx context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	if _, err := h.requireCaller(ctx); err != nil {
		return nil, err
	}
	if err := h.requireStore(); err != nil {
		return nil, err
	}

	id, accessHash, ok := locationID(req.Location)
	if !ok {
		return nil, tgerr.New(400, "LOCATION_INVALID")
	}
	file, ok, err := h.db.FileByID(ctx, id)
	if err != nil {
		return nil, h.internal(ctx, "file lookup", err)
	}
	if !ok || file.AccessHash != accessHash {
		return nil, tgerr.New(400, "FILE_ID_INVALID")
	}

	rc, err := h.store.GetRange(ctx, file.ObjectKey, req.Offset, int64(req.Limit))
	if err != nil {
		return nil, h.internal(ctx, "get range", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, h.internal(ctx, "read", err)
	}

	return &tg.UploadFile{Type: &tg.StorageFilePartial{}, Bytes: data}, nil
}

// photoMedia builds a MessageMediaPhoto for stored media.
func photoMedia(f teled.File) *tg.MessageMediaPhoto {
	photo := &tg.Photo{
		ID:            f.ID,
		AccessHash:    f.AccessHash,
		FileReference: f.FileReference,
		Date:          int(f.CreatedAt.Unix()),
		DCID:          1, // single-DC v1.
		Sizes:         []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 1, H: 1, Size: int(f.Size)}},
	}
	photo.SetFlags()
	m := &tg.MessageMediaPhoto{}
	m.SetPhoto(photo)
	m.SetFlags()
	return m
}

func uploadedFileID(media tg.InputMediaClass) (int64, bool) {
	switch m := media.(type) {
	case *tg.InputMediaUploadedPhoto:
		return inputFileID(m.File)
	case *tg.InputMediaUploadedDocument:
		return inputFileID(m.File)
	default:
		return 0, false
	}
}

func inputFileID(f tg.InputFileClass) (int64, bool) {
	switch v := f.(type) {
	case *tg.InputFile:
		return v.ID, true
	case *tg.InputFileBig:
		return v.ID, true
	default:
		return 0, false
	}
}

func locationID(loc tg.InputFileLocationClass) (id, accessHash int64, ok bool) {
	switch v := loc.(type) {
	case *tg.InputPhotoFileLocation:
		return v.ID, v.AccessHash, true
	case *tg.InputDocumentFileLocation:
		return v.ID, v.AccessHash, true
	default:
		return 0, 0, false
	}
}
