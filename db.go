package teled

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrMessageID indicates a referenced message does not exist or is not editable
// by the caller. DB implementations return it from EditMessage so callers can
// translate it into the appropriate RPC error.
var ErrMessageID = errors.New("message id invalid")

// Update log entry types. UpdateLogEntry.Type holds one of these, telling
// consumers how to interpret the entry (and its Extra payload).
const (
	UpdateNew       = "new"       // a new message; GlobalID set, no Extra
	UpdateEdit      = "edit"      // an edited message; GlobalID set, no Extra
	UpdateDelete    = "delete"    // deleted messages; Extra carries local ids
	UpdateReadInbox = "readinbox" // inbox read; Extra carries peer and max id
)

// EncodeDeleted encodes the local ids of a delete update into an UpdateLogEntry
// Extra payload. DecodeDeleted is its inverse.
func EncodeDeleted(ids []int64) []byte {
	b, _ := json.Marshal(deleteExtra{IDs: ids})
	return b
}

// DecodeDeleted extracts deleted local ids from a delete log entry's Extra.
func DecodeDeleted(extra []byte) []int64 {
	var d deleteExtra

	_ = json.Unmarshal(extra, &d)

	return d.IDs
}

// EncodeRead encodes the (peer, maxID) of a read update into an UpdateLogEntry
// Extra payload. DecodeRead is its inverse.
func EncodeRead(peer, maxID int64) []byte {
	b, _ := json.Marshal(readExtra{Peer: peer, MaxID: maxID})
	return b
}

// DecodeRead extracts (peer, maxID) from a read log entry's Extra.
func DecodeRead(extra []byte) (peer, maxID int64) {
	var r readExtra

	_ = json.Unmarshal(extra, &r)

	return r.Peer, r.MaxID
}

// deleteExtra is the Extra payload for a delete update.
type deleteExtra struct {
	IDs []int64 `json:"ids"`
}

// readExtra is the Extra payload for a read update.
type readExtra struct {
	Peer  int64 `json:"peer"`
	MaxID int64 `json:"max_id"`
}

// DB is teled's storage port: everything the server persists, behind one
// interface so backends are interchangeable. The PostgreSQL implementation
// lives in internal/db; an in-memory implementation (for tests and embedding
// without a database) lives in github.com/gotd/teled/memory.
//
// All ids are teled account ids. Message ids come in two flavors: a global id
// is the canonical id shared by both participants of a DM, while a local id is
// a participant's own per-account message id (what Telegram clients see).
type DB interface {
	// Ready reports whether the storage is reachable.
	Ready(ctx context.Context) error

	// CreateUser inserts a new human user with the given phone and name.
	CreateUser(ctx context.Context, phone, firstName, lastName string) (User, error)
	// CreateBot inserts a new bot account authenticated by token. username may
	// be empty.
	CreateBot(ctx context.Context, token, username, firstName string) (User, error)
	// CreateOwnedBot creates a bot owned by ownerID, minting its token as
	// "<bot_id>:<secret>" once the id is known, and returns the persisted
	// account with BotToken populated.
	CreateOwnedBot(ctx context.Context, username, firstName string, ownerID int64, secret string) (User, error)
	// SetBotToken replaces a bot's auth token. A missing bot is a no-op.
	SetBotToken(ctx context.Context, botID int64, token string) error
	// BotByToken returns the bot account holding token, if any.
	BotByToken(ctx context.Context, token string) (*User, bool, error)
	// BotsByOwner returns the bots created by ownerID, oldest first.
	BotsByOwner(ctx context.Context, ownerID int64) ([]User, error)
	// UserByID returns a user by id.
	UserByID(ctx context.Context, id int64) (*User, bool, error)
	// UserByPhone returns a user by phone number.
	UserByPhone(ctx context.Context, phone string) (*User, bool, error)
	// UserByUsername returns a user by username.
	UserByUsername(ctx context.Context, username string) (*User, bool, error)
	// SetUsername sets (or, when username is empty, clears) a user's username and
	// returns the updated account. Callers are responsible for enforcing
	// uniqueness; backends may also reject a clash via a constraint error.
	SetUsername(ctx context.Context, userID int64, username string) (User, error)
	// UsersByIDs returns the users with the given ids, in arbitrary order.
	UsersByIDs(ctx context.Context, ids []int64) ([]User, error)

	// SavePhoneCode stores (or replaces) a login code for phone under codeHash,
	// valid for ttl.
	SavePhoneCode(ctx context.Context, phone, codeHash, code string, ttl time.Duration) error
	// PhoneCode returns the unexpired code stored for (phone, codeHash).
	PhoneCode(ctx context.Context, phone, codeHash string) (code string, ok bool, err error)

	// BindSession binds an MTProto auth key to a logged-in user.
	BindSession(ctx context.Context, keyID [8]byte, userID int64) error
	// SessionUserID returns the user bound to the given auth key, if any.
	SessionUserID(ctx context.Context, keyID [8]byte) (userID int64, ok bool, err error)
	// Unbind removes the user binding for an auth key (logout).
	Unbind(ctx context.Context, keyID [8]byte) error

	// BindTempAuthKey records that a temporary (PFS) auth key maps to a permanent
	// one (auth.bindTempAuthKey), valid until expiresAt. Requests on the temp key
	// are then resolved to the permanent key for session/user lookup, so the
	// authorization survives temp-key rotation and restarts.
	BindTempAuthKey(ctx context.Context, tempKeyID, permKeyID [8]byte, expiresAt time.Time) error
	// PermAuthKey returns the permanent key a temporary key is bound to, when the
	// binding exists and has not expired.
	PermAuthKey(ctx context.Context, tempKeyID [8]byte) (permKeyID [8]byte, ok bool, err error)

	// SendMessage persists a DM atomically: the canonical message plus a
	// per-account ref (with its own local id and pts) for sender and recipient.
	// A non-zero mediaFileID attaches stored media.
	SendMessage(ctx context.Context, fromID, peerID int64, text string, randomID, mediaFileID int64) (SentMessage, error)
	// GetHistory returns up to limit messages between self and peer, newest
	// first. When offsetID > 0 only messages with a smaller local id return.
	GetHistory(ctx context.Context, self, peer, offsetID int64, limit int) ([]Message, error)
	// EditMessage updates the text of a message the caller sent, returning the
	// data needed to emit edit updates to both participants. It returns
	// ErrMessageID when the message does not exist or the caller is not its
	// sender.
	EditMessage(ctx context.Context, self, localID int64, text string) (EditResult, error)
	// DeleteMessages marks the caller's messages (by local id) deleted and
	// allocates a pts covering them.
	DeleteMessages(ctx context.Context, self int64, localIDs []int64) (DeleteResult, error)
	// MessageByGlobal returns the caller's view of a message by its canonical id.
	MessageByGlobal(ctx context.Context, self, globalID int64) (Message, bool, error)

	// GetDialogs returns the caller's conversations, newest activity first.
	GetDialogs(ctx context.Context, self int64, limit int) ([]Dialog, error)
	// ReadHistory marks the caller's incoming messages from peer up to maxID as
	// read and allocates a pts for the read event.
	ReadHistory(ctx context.Context, self, peer, maxID int64) (pts int, err error)

	// GetState returns the caller's current update state.
	GetState(ctx context.Context, self int64) (State, error)
	// GetDifference returns log entries with pts greater than sincePts (up to
	// limit) and the caller's current pts.
	GetDifference(ctx context.Context, self int64, sincePts, limit int) ([]UpdateLogEntry, int, error)

	// SaveFile records uploaded media, generating an access hash and file
	// reference, and returns the populated File.
	SaveFile(ctx context.Context, f File) (File, error)
	// FileByID returns stored media by id.
	FileByID(ctx context.Context, id int64) (File, bool, error)

	// BotFatherState returns the user's pending BotFather flow. A zero-value
	// state (empty Step) and no error means no flow is in progress.
	BotFatherState(ctx context.Context, userID int64) (BotFatherState, error)
	// SetBotFatherState upserts the user's pending BotFather flow.
	SetBotFatherState(ctx context.Context, userID int64, s BotFatherState) error
	// ClearBotFatherState ends any pending BotFather flow for the user.
	ClearBotFatherState(ctx context.Context, userID int64) error

	// SetBotCommands replaces the command list a bot publishes for the given
	// scope and language. A nil commands slice stores an empty list (distinct
	// from having no row, which ResetBotCommands produces).
	SetBotCommands(ctx context.Context, botUserID int64, scope, langCode string, commands []BotCommand) error
	// BotCommands returns the command list published for the given scope and
	// language. A missing row yields an empty slice and no error.
	BotCommands(ctx context.Context, botUserID int64, scope, langCode string) ([]BotCommand, error)
	// ResetBotCommands removes the command list for the given scope and
	// language, restoring the bot's default (empty) commands there.
	ResetBotCommands(ctx context.Context, botUserID int64, scope, langCode string) error
}
