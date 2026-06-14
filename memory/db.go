package memory

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// DB is an in-memory teled.DB. It mirrors the semantics of the PostgreSQL
// backend in internal/db (per-account local message ids and pts, a shared
// canonical message per DM, a durable update log) without any external storage.
// A single mutex guards all state; every method takes it, so operations are
// serialized and therefore atomic, matching the transactional backend.
type DB struct {
	mu sync.Mutex

	users   map[int64]*memUser
	userSeq int64

	messages map[int64]*memMessage // global id -> canonical message
	msgSeq   int64

	// refs is the per-account message view: user id -> local id -> ref.
	refs map[int64]map[int64]*memRef

	// updates is the per-account durable update log, ordered by append (and
	// therefore by pts, which is monotonic per account).
	updates map[int64][]teled.UpdateLogEntry

	files   map[int64]*teled.File
	fileSeq int64

	sessions   map[[8]byte]int64
	tempKeys   map[[8]byte]memTempKey
	phoneCodes map[string]memPhoneCode
	drafts     map[int64]map[int64]teled.Draft   // userID -> peerID -> draft
	contacts   map[int64]map[int64]teled.Contact // ownerID -> contactID -> contact
	botStates  map[int64]teled.BotFatherState
	// botCommands is keyed by botID|scope|lang; a present (possibly empty) value
	// is distinct from an absent key, just as a stored row differs from no row.
	botCommands map[string][]teled.BotCommand
}

// memUser is a stored user plus the per-account counters and ownership link the
// public teled.User type does not carry.
type memUser struct {
	u             teled.User
	pts           int
	lastMessageID int64
	botOwnerID    int64 // 0 when the bot was not created through BotFather
}

// memMessage is a canonical message shared by both participants of a DM.
type memMessage struct {
	globalID    int64
	from        int64
	peer        int64
	text        string
	date        time.Time
	editDate    time.Time // zero when never edited
	randomID    int64
	mediaFileID int64 // 0 when text-only
	deleted     bool
}

// memRef is one participant's view of a canonical message.
type memRef struct {
	localID  int64
	globalID int64
	out      bool
	unread   bool
}

type memPhoneCode struct {
	code      string
	expiresAt time.Time
}

type memTempKey struct {
	perm      [8]byte
	expiresAt time.Time
}

var _ teled.DB = (*DB)(nil)

// NewDB creates an empty in-memory database with the built-in BotFather account
// seeded, matching the PostgreSQL backend after migration.
func NewDB() *DB {
	d := &DB{
		users:       make(map[int64]*memUser),
		messages:    make(map[int64]*memMessage),
		refs:        make(map[int64]map[int64]*memRef),
		updates:     make(map[int64][]teled.UpdateLogEntry),
		files:       make(map[int64]*teled.File),
		sessions:    make(map[[8]byte]int64),
		tempKeys:    make(map[[8]byte]memTempKey),
		phoneCodes:  make(map[string]memPhoneCode),
		drafts:      make(map[int64]map[int64]teled.Draft),
		contacts:    make(map[int64]map[int64]teled.Contact),
		botStates:   make(map[int64]teled.BotFatherState),
		botCommands: make(map[string][]teled.BotCommand),
	}
	// Seed BotFather with the same fixed id and access hash as migration
	// 000008, so clients can re-resolve it deterministically.
	d.users[teled.BotFatherID] = &memUser{u: teled.User{
		ID:         teled.BotFatherID,
		AccessHash: 7264819913547,
		Username:   "BotFather",
		FirstName:  "BotFather",
		IsBot:      true,
		CreatedAt:  time.Now(),
	}}

	return d
}

// Ready reports whether the database is reachable. The in-memory DB is always
// ready.
func (d *DB) Ready(context.Context) error { return nil }

// --- users ---------------------------------------------------------------

// genAccessHash returns a random non-zero access hash, like the SQL backend.
func genAccessHash() int64 {
	var b [8]byte

	_, _ = rand.Read(b[:])

	h := int64(binary.LittleEndian.Uint64(b[:])) // #nosec G115 -- bit reinterpretation.
	if h == 0 {
		h = 1
	}

	return h
}

// userCopy returns a heap copy of u so callers cannot mutate stored state.
func userCopy(u teled.User) *teled.User {
	c := u
	return &c
}

func (d *DB) phoneTaken(phone string) bool {
	if phone == "" {
		return false
	}

	for _, mu := range d.users {
		if mu.u.Phone == phone {
			return true
		}
	}

	return false
}

func (d *DB) usernameTaken(username string) bool {
	if username == "" {
		return false
	}

	for _, mu := range d.users {
		if mu.u.Username == username {
			return true
		}
	}

	return false
}

func (d *DB) tokenTaken(token string) bool {
	if token == "" {
		return false
	}

	for _, mu := range d.users {
		if mu.u.BotToken == token {
			return true
		}
	}

	return false
}

// CreateUser inserts a new human user with the given phone and name.
func (d *DB) CreateUser(_ context.Context, phone, firstName, lastName string) (teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.phoneTaken(phone) {
		return teled.User{}, errors.Errorf("phone %q already exists", phone)
	}

	d.userSeq++
	u := teled.User{
		ID:         d.userSeq,
		AccessHash: genAccessHash(),
		Phone:      phone,
		FirstName:  firstName,
		LastName:   lastName,
		CreatedAt:  time.Now(),
	}
	d.users[u.ID] = &memUser{u: u}

	return u, nil
}

// CreateBot inserts a new bot account authenticated by token. username may be
// empty.
func (d *DB) CreateBot(_ context.Context, token, username, firstName string) (teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.tokenTaken(token) {
		return teled.User{}, errors.Errorf("bot token already exists")
	}

	if d.usernameTaken(username) {
		return teled.User{}, errors.Errorf("username %q already exists", username)
	}

	d.userSeq++
	u := teled.User{
		ID:         d.userSeq,
		AccessHash: genAccessHash(),
		Username:   username,
		FirstName:  firstName,
		IsBot:      true,
		BotToken:   token,
		CreatedAt:  time.Now(),
	}
	d.users[u.ID] = &memUser{u: u}

	return u, nil
}

// CreateOwnedBot creates a bot owned by ownerID, minting its token as
// "<bot_id>:<secret>" once the id is known.
func (d *DB) CreateOwnedBot(_ context.Context, username, firstName string, ownerID int64, secret string) (teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.usernameTaken(username) {
		return teled.User{}, errors.Errorf("username %q already exists", username)
	}

	d.userSeq++
	id := d.userSeq
	u := teled.User{
		ID:         id,
		AccessHash: genAccessHash(),
		Username:   username,
		FirstName:  firstName,
		IsBot:      true,
		BotToken:   strconv.FormatInt(id, 10) + ":" + secret,
		CreatedAt:  time.Now(),
	}
	d.users[id] = &memUser{u: u, botOwnerID: ownerID}

	return u, nil
}

// SetBotToken replaces a bot's auth token. A missing or non-bot user is a no-op.
func (d *DB) SetBotToken(_ context.Context, botID int64, token string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if mu, ok := d.users[botID]; ok && mu.u.IsBot {
		mu.u.BotToken = token
	}

	return nil
}

// BotByToken returns the bot account holding token, if any.
func (d *DB) BotByToken(_ context.Context, token string) (*teled.User, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, mu := range d.users {
		if mu.u.IsBot && mu.u.BotToken == token {
			return userCopy(mu.u), true, nil
		}
	}

	return nil, false, nil
}

// BotsByOwner returns the bots created by ownerID, oldest first.
func (d *DB) BotsByOwner(_ context.Context, ownerID int64) ([]teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var bots []teled.User

	for _, mu := range d.users {
		if mu.botOwnerID == ownerID {
			bots = append(bots, mu.u)
		}
	}

	sort.Slice(bots, func(i, j int) bool { return bots[i].ID < bots[j].ID })

	return bots, nil
}

// UserByID returns a user by id.
func (d *DB) UserByID(_ context.Context, id int64) (*teled.User, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if mu, ok := d.users[id]; ok {
		return userCopy(mu.u), true, nil
	}

	return nil, false, nil
}

// UserByPhone returns a user by phone number.
func (d *DB) UserByPhone(_ context.Context, phone string) (*teled.User, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, mu := range d.users {
		if mu.u.Phone == phone {
			return userCopy(mu.u), true, nil
		}
	}

	return nil, false, nil
}

// UserByUsername returns a user by username.
func (d *DB) UserByUsername(_ context.Context, username string) (*teled.User, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, mu := range d.users {
		if mu.u.Username == username {
			return userCopy(mu.u), true, nil
		}
	}

	return nil, false, nil
}

// SetProfile updates the provided profile fields and returns the updated user.
// A nil pointer leaves that field unchanged.
func (d *DB) SetProfile(_ context.Context, userID int64, firstName, lastName, about *string) (teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	mu, ok := d.users[userID]
	if !ok {
		return teled.User{}, errors.Errorf("user %d not found", userID)
	}

	if firstName != nil {
		mu.u.FirstName = *firstName
	}

	if lastName != nil {
		mu.u.LastName = *lastName
	}

	if about != nil {
		mu.u.About = *about
	}

	return *userCopy(mu.u), nil
}

// SearchUsers returns users matching query by username prefix or name
// substring, case-insensitively, ordered by id, up to limit.
func (d *DB) SearchUsers(_ context.Context, query string, limit int) ([]teled.User, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var out []teled.User

	for _, mu := range d.users {
		u := mu.u
		if strings.HasPrefix(strings.ToLower(u.Username), query) ||
			strings.Contains(strings.ToLower(u.FirstName), query) ||
			strings.Contains(strings.ToLower(u.LastName), query) {
			out = append(out, u)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	if len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

// SetUsername sets (or clears, when empty) a user's username and returns the
// updated account. It rejects a username already taken by another user.
func (d *DB) SetUsername(_ context.Context, userID int64, username string) (teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	mu, ok := d.users[userID]
	if !ok {
		return teled.User{}, errors.Errorf("user %d not found", userID)
	}

	if username != "" {
		for id, other := range d.users {
			if id != userID && other.u.Username == username {
				return teled.User{}, errors.Errorf("username %q already exists", username)
			}
		}
	}

	mu.u.Username = username

	return *userCopy(mu.u), nil
}

// UsersByIDs returns the users with the given ids, in arbitrary order.
func (d *DB) UsersByIDs(_ context.Context, ids []int64) ([]teled.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var users []teled.User
	for _, id := range ids {
		if mu, ok := d.users[id]; ok {
			users = append(users, mu.u)
		}
	}

	return users, nil
}

// --- phone codes ---------------------------------------------------------

func phoneCodeKey(phone, codeHash string) string { return phone + "\x00" + codeHash }

// SavePhoneCode stores (or replaces) a login code valid for ttl.
func (d *DB) SavePhoneCode(_ context.Context, phone, codeHash, code string, ttl time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.phoneCodes[phoneCodeKey(phone, codeHash)] = memPhoneCode{code: code, expiresAt: time.Now().Add(ttl)}

	return nil
}

// PhoneCode returns the unexpired code stored for (phone, codeHash).
func (d *DB) PhoneCode(_ context.Context, phone, codeHash string) (code string, ok bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pc, ok := d.phoneCodes[phoneCodeKey(phone, codeHash)]
	if !ok || !pc.expiresAt.After(time.Now()) {
		return "", false, nil
	}

	return pc.code, true, nil
}

// --- sessions ------------------------------------------------------------

// BindSession binds an MTProto auth key to a logged-in user.
func (d *DB) BindSession(_ context.Context, keyID [8]byte, userID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sessions[keyID] = userID

	return nil
}

// SessionUserID returns the user bound to the given auth key, if any.
func (d *DB) SessionUserID(_ context.Context, keyID [8]byte) (userID int64, ok bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	userID, ok = d.sessions[keyID]

	return userID, ok, nil
}

// Unbind removes the user binding for an auth key (logout).
func (d *DB) Unbind(_ context.Context, keyID [8]byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.sessions, keyID)

	return nil
}

func (d *DB) BindTempAuthKey(_ context.Context, tempKeyID, permKeyID [8]byte, expiresAt time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tempKeys[tempKeyID] = memTempKey{perm: permKeyID, expiresAt: expiresAt}

	return nil
}

func (d *DB) PermAuthKey(_ context.Context, tempKeyID [8]byte) (permKeyID [8]byte, ok bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tk, ok := d.tempKeys[tempKeyID]
	if !ok || !tk.expiresAt.After(time.Now()) {
		return [8]byte{}, false, nil
	}

	return tk.perm, true, nil
}

// --- messages ------------------------------------------------------------

// allocate bumps the user's local message id and pts together. Caller holds mu.
func (d *DB) allocate(userID int64) (localID int64, pts int) {
	mu := d.users[userID]
	mu.lastMessageID++
	mu.pts++

	return mu.lastMessageID, mu.pts
}

// allocatePts advances only the user's pts by count. Caller holds mu.
func (d *DB) allocatePts(userID int64, count int) int {
	mu := d.users[userID]
	mu.pts += count

	return mu.pts
}

// putRef stores a participant's view of a message. Caller holds mu.
func (d *DB) putRef(userID int64, r *memRef) {
	m := d.refs[userID]
	if m == nil {
		m = make(map[int64]*memRef)
		d.refs[userID] = m
	}

	m[r.localID] = r
}

// logUpdate appends one entry to a user's update log. Caller holds mu.
func (d *DB) logUpdate(userID int64, pts, ptsCount int, typ string, globalID *int64, extra []byte) {
	d.updates[userID] = append(d.updates[userID], teled.UpdateLogEntry{
		Pts:      pts,
		PtsCount: ptsCount,
		Type:     typ,
		GlobalID: globalID,
		Extra:    extra,
		Date:     time.Now(),
	})
}

// refByGlobal finds a user's ref for a canonical message. Caller holds mu.
func (d *DB) refByGlobal(userID, globalID int64) (*memRef, bool) {
	for _, r := range d.refs[userID] {
		if r.globalID == globalID {
			return r, true
		}
	}

	return nil, false
}

// SendMessage persists a DM: the canonical message plus a per-account ref for
// sender and recipient, each with its own local id, pts and log entry.
func (d *DB) SendMessage(_ context.Context, fromID, peerID int64, text string, randomID, mediaFileID int64) (teled.SentMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var sent teled.SentMessage

	sent.SelfChat = fromID == peerID

	d.msgSeq++
	m := &memMessage{
		globalID:    d.msgSeq,
		from:        fromID,
		peer:        peerID,
		text:        text,
		date:        time.Now(),
		randomID:    randomID,
		mediaFileID: mediaFileID,
	}
	d.messages[m.globalID] = m
	sent.GlobalID = m.globalID
	sent.Date = m.date

	sent.SenderLocalID, sent.SenderPts = d.allocate(fromID)
	d.putRef(fromID, &memRef{localID: sent.SenderLocalID, globalID: m.globalID, out: true, unread: false})
	gid := m.globalID
	d.logUpdate(fromID, sent.SenderPts, 1, teled.UpdateNew, &gid, nil)

	if !sent.SelfChat {
		sent.RecipientLocalID, sent.RecipientPts = d.allocate(peerID)
		d.putRef(peerID, &memRef{localID: sent.RecipientLocalID, globalID: m.globalID, out: false, unread: true})
		d.logUpdate(peerID, sent.RecipientPts, 1, teled.UpdateNew, &gid, nil)
	}

	return sent, nil
}

// other returns the conversation partner of self for a canonical message.
func other(self int64, m *memMessage) int64 {
	if m.from == self {
		return m.peer
	}

	return m.from
}

// message builds the viewer-relative teled.Message for a ref. Caller holds mu.
func (d *DB) message(self int64, r *memRef, m *memMessage) teled.Message {
	msg := teled.Message{
		GlobalID:   m.globalID,
		LocalID:    r.localID,
		FromUserID: m.from,
		PeerUserID: other(self, m),
		Out:        r.out,
		Text:       m.text,
		Date:       m.date,
		EditDate:   m.editDate,
		RandomID:   m.randomID,
	}

	if m.mediaFileID != 0 {
		if f, ok := d.files[m.mediaFileID]; ok {
			fc := *f
			msg.Media = &fc
		}
	}

	return msg
}

// GetHistory returns up to limit messages between self and peer, newest first.
func (d *DB) GetHistory(_ context.Context, self, peer, offsetID int64, limit int) ([]teled.Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var msgs []teled.Message

	for _, r := range d.refs[self] {
		m := d.messages[r.globalID]
		if m == nil || m.deleted || other(self, m) != peer {
			continue
		}

		if offsetID > 0 && r.localID >= offsetID {
			continue
		}

		msgs = append(msgs, d.message(self, r, m))
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].LocalID > msgs[j].LocalID })

	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}

	return msgs, nil
}

// AddContact saves (or updates) contactID in ownerID's contact list.
func (d *DB) AddContact(_ context.Context, ownerID, contactID int64, firstName, lastName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.contacts[ownerID] == nil {
		d.contacts[ownerID] = map[int64]teled.Contact{}
	}

	d.contacts[ownerID][contactID] = teled.Contact{UserID: contactID, FirstName: firstName, LastName: lastName}

	return nil
}

// DeleteContacts removes the given users from ownerID's contact list.
func (d *DB) DeleteContacts(_ context.Context, ownerID int64, contactIDs []int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, id := range contactIDs {
		delete(d.contacts[ownerID], id)
	}

	return nil
}

// Contacts returns ownerID's saved contacts, ordered by user id.
func (d *DB) Contacts(_ context.Context, ownerID int64) ([]teled.Contact, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var contacts []teled.Contact
	for _, c := range d.contacts[ownerID] {
		contacts = append(contacts, c)
	}

	sort.Slice(contacts, func(i, j int) bool { return contacts[i].UserID < contacts[j].UserID })

	return contacts, nil
}

// SaveDraft stores (or clears, when blank) the caller's draft for a peer.
func (d *DB) SaveDraft(_ context.Context, userID, peerID int64, text string) (time.Time, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		delete(d.drafts[userID], peerID)
		return time.Time{}, nil
	}

	date := time.Now()

	if d.drafts[userID] == nil {
		d.drafts[userID] = map[int64]teled.Draft{}
	}

	d.drafts[userID][peerID] = teled.Draft{PeerUserID: peerID, Text: text, Date: date}

	return date, nil
}

// Drafts returns all of the caller's saved drafts, newest first.
func (d *DB) Drafts(_ context.Context, userID int64) ([]teled.Draft, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var drafts []teled.Draft
	for _, dr := range d.drafts[userID] {
		drafts = append(drafts, dr)
	}

	sort.Slice(drafts, func(i, j int) bool { return drafts[i].Date.After(drafts[j].Date) })

	return drafts, nil
}

// SearchMessages returns the caller's messages with peer whose text matches
// query (case-insensitive substring), newest first, up to limit.
func (d *DB) SearchMessages(_ context.Context, self, peer int64, query string, limit int) ([]teled.Message, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}

	if limit <= 0 || limit > 100 {
		limit = 100
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var msgs []teled.Message

	for _, r := range d.refs[self] {
		m := d.messages[r.globalID]
		if m == nil || m.deleted || other(self, m) != peer {
			continue
		}

		if !strings.Contains(strings.ToLower(m.text), query) {
			continue
		}

		msgs = append(msgs, d.message(self, r, m))
	}

	sort.Slice(msgs, func(i, j int) bool { return msgs[i].LocalID > msgs[j].LocalID })

	if len(msgs) > limit {
		msgs = msgs[:limit]
	}

	return msgs, nil
}

// MessageByGlobal returns the caller's view of a message by its canonical id.
func (d *DB) MessageByGlobal(_ context.Context, self, globalID int64) (teled.Message, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	r, ok := d.refByGlobal(self, globalID)
	if !ok {
		return teled.Message{}, false, nil
	}

	m := d.messages[globalID]
	if m == nil {
		return teled.Message{}, false, nil
	}

	return d.message(self, r, m), true, nil
}

// EditMessage updates the text of a message the caller sent.
func (d *DB) EditMessage(_ context.Context, self, localID int64, text string) (teled.EditResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	r, ok := d.refs[self][localID]
	if !ok {
		return teled.EditResult{}, teled.ErrMessageID
	}

	m := d.messages[r.globalID]
	if m == nil || m.deleted || m.from != self {
		return teled.EditResult{}, teled.ErrMessageID
	}

	m.text = text
	m.editDate = time.Now()
	res := teled.EditResult{
		SelfLocalID: localID,
		Date:        m.date,
		EditDate:    m.editDate,
		PeerUserID:  m.peer,
	}
	res.SelfPts = d.allocatePts(self, 1)
	gid := m.globalID
	d.logUpdate(self, res.SelfPts, 1, teled.UpdateEdit, &gid, nil)

	if m.peer != self {
		if pr, ok := d.refByGlobal(m.peer, m.globalID); ok {
			res.PeerLocalID = pr.localID
		}

		res.PeerPts = d.allocatePts(m.peer, 1)
		d.logUpdate(m.peer, res.PeerPts, 1, teled.UpdateEdit, &gid, nil)
	}

	return res, nil
}

// DeleteMessages marks the caller's messages (by local id) deleted and
// allocates a pts covering them.
func (d *DB) DeleteMessages(_ context.Context, self int64, localIDs []int64) (teled.DeleteResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var deleted []int64

	for _, localID := range localIDs {
		r, ok := d.refs[self][localID]
		if !ok {
			continue
		}

		if m := d.messages[r.globalID]; m != nil {
			m.deleted = true
		}

		deleted = append(deleted, localID)
	}

	res := teled.DeleteResult{LocalIDs: deleted, PtsCount: len(deleted)}
	if len(deleted) > 0 {
		res.Pts = d.allocatePts(self, len(deleted))
		d.logUpdate(self, res.Pts, len(deleted), teled.UpdateDelete, nil, teled.EncodeDeleted(deleted))
	}

	return res, nil
}

// --- dialogs -------------------------------------------------------------

// GetDialogs returns the caller's conversations, newest activity first.
func (d *DB) GetDialogs(_ context.Context, self int64, limit int) ([]teled.Dialog, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// peerReadGlobal reports whether peerID has read the message with globalID
	// (their incoming copy is no longer unread).
	peerReadGlobal := func(peerID, globalID int64) bool {
		for _, pr := range d.refs[peerID] {
			if pr.globalID == globalID {
				return !pr.out && !pr.unread
			}
		}

		return false
	}

	byPeer := make(map[int64]*teled.Dialog)

	for _, r := range d.refs[self] {
		m := d.messages[r.globalID]
		if m == nil || m.deleted {
			continue
		}

		peer := other(self, m)
		dl := byPeer[peer]

		if dl == nil {
			dl = &teled.Dialog{PeerUserID: peer}
			byPeer[peer] = dl
		}

		if r.localID > dl.TopMessageID {
			dl.TopMessageID = r.localID
		}

		if r.unread && !r.out {
			dl.UnreadCount++
		}

		if !r.out && !r.unread && r.localID > dl.ReadInboxMaxID {
			dl.ReadInboxMaxID = r.localID
		}

		if r.out && r.localID > dl.ReadOutboxMaxID && peerReadGlobal(peer, r.globalID) {
			dl.ReadOutboxMaxID = r.localID
		}
	}

	dialogs := make([]teled.Dialog, 0, len(byPeer))
	for _, dl := range byPeer {
		dialogs = append(dialogs, *dl)
	}

	sort.Slice(dialogs, func(i, j int) bool { return dialogs[i].TopMessageID > dialogs[j].TopMessageID })

	if limit > 0 && len(dialogs) > limit {
		dialogs = dialogs[:limit]
	}

	return dialogs, nil
}

// ReadHistory marks the caller's incoming messages from peer up to maxID as read
// and allocates a pts for the read event.
func (d *DB) ReadHistory(_ context.Context, self, peer, maxID int64) (teled.ReadResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var readGlobals []int64

	for _, r := range d.refs[self] {
		if r.out || r.localID > maxID {
			continue
		}

		m := d.messages[r.globalID]
		if m == nil || other(self, m) != peer {
			continue
		}

		r.unread = false
		readGlobals = append(readGlobals, r.globalID)
	}

	var res teled.ReadResult

	// Peer's max outgoing local id among the read messages, for the receipt.
	readSet := make(map[int64]bool, len(readGlobals))
	for _, g := range readGlobals {
		readSet[g] = true
	}

	for _, pr := range d.refs[peer] {
		if pr.out && readSet[pr.globalID] && pr.localID > res.OutboxMaxID {
			res.OutboxMaxID = pr.localID
		}
	}

	// Remaining unread incoming with this peer.
	for _, r := range d.refs[self] {
		if r.out || !r.unread {
			continue
		}

		if m := d.messages[r.globalID]; m != nil && other(self, m) == peer {
			res.UnreadCount++
		}
	}

	res.InboxPts = d.allocatePts(self, 1)
	d.logUpdate(self, res.InboxPts, 1, teled.UpdateReadInbox, nil, teled.EncodeRead(peer, maxID))

	if res.OutboxMaxID > 0 && peer != self {
		res.OutboxPts = d.allocatePts(peer, 1)
		d.logUpdate(peer, res.OutboxPts, 1, teled.UpdateReadOutbox, nil, teled.EncodeRead(self, res.OutboxMaxID))
	}

	return res, nil
}

// --- updates -------------------------------------------------------------

// GetState returns the caller's current update state.
func (d *DB) GetState(_ context.Context, self int64) (teled.State, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	mu, ok := d.users[self]
	if !ok {
		return teled.State{}, errors.Errorf("user %d not found", self)
	}

	return teled.State{Pts: mu.pts, Date: time.Now()}, nil
}

// GetDifference returns log entries with pts greater than sincePts (up to limit)
// and the caller's current pts.
func (d *DB) GetDifference(_ context.Context, self int64, sincePts, limit int) ([]teled.UpdateLogEntry, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	mu, ok := d.users[self]
	if !ok {
		return nil, 0, errors.Errorf("user %d not found", self)
	}

	var entries []teled.UpdateLogEntry

	for _, e := range d.updates[self] {
		if e.Pts > sincePts {
			entries = append(entries, e)
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Pts < entries[j].Pts })

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, mu.pts, nil
}

// --- files ---------------------------------------------------------------

// SaveFile records uploaded media, generating an access hash and file reference.
func (d *DB) SaveFile(_ context.Context, f teled.File) (teled.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	f.AccessHash = genAccessHash()
	f.FileReference = make([]byte, 16)

	if _, err := rand.Read(f.FileReference); err != nil {
		return teled.File{}, errors.Wrap(err, "rand")
	}

	if f.Kind == "" {
		f.Kind = "photo"
	}

	d.fileSeq++
	f.ID = d.fileSeq
	f.CreatedAt = time.Now()

	stored := f
	d.files[f.ID] = &stored

	return f, nil
}

// FileByID returns stored media by id.
func (d *DB) FileByID(_ context.Context, id int64) (teled.File, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if f, ok := d.files[id]; ok {
		return *f, true, nil
	}

	return teled.File{}, false, nil
}

// --- botfather -----------------------------------------------------------

// BotFatherState returns the user's pending BotFather flow.
func (d *DB) BotFatherState(_ context.Context, userID int64) (teled.BotFatherState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.botStates[userID], nil
}

// SetBotFatherState upserts the user's pending BotFather flow.
func (d *DB) SetBotFatherState(_ context.Context, userID int64, s teled.BotFatherState) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.botStates[userID] = s

	return nil
}

// ClearBotFatherState ends any pending BotFather flow for the user.
func (d *DB) ClearBotFatherState(_ context.Context, userID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.botStates, userID)

	return nil
}

// --- bot commands --------------------------------------------------------

func botCommandsKey(botUserID int64, scope, langCode string) string {
	return strconv.FormatInt(botUserID, 10) + "\x00" + scope + "\x00" + langCode
}

// SetBotCommands replaces the command list a bot publishes for the given scope
// and language. A nil slice stores an empty list (distinct from no row).
func (d *DB) SetBotCommands(_ context.Context, botUserID int64, scope, langCode string, commands []teled.BotCommand) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	stored := make([]teled.BotCommand, len(commands))
	copy(stored, commands)
	d.botCommands[botCommandsKey(botUserID, scope, langCode)] = stored

	return nil
}

// BotCommands returns the command list published for the given scope and
// language. A missing row yields a nil slice and no error.
func (d *DB) BotCommands(_ context.Context, botUserID int64, scope, langCode string) ([]teled.BotCommand, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	stored, ok := d.botCommands[botCommandsKey(botUserID, scope, langCode)]
	if !ok {
		return nil, nil
	}

	out := make([]teled.BotCommand, len(stored))
	copy(out, stored)

	return out, nil
}

// ResetBotCommands removes the command list for the given scope and language.
func (d *DB) ResetBotCommands(_ context.Context, botUserID int64, scope, langCode string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.botCommands, botCommandsKey(botUserID, scope, langCode))

	return nil
}
