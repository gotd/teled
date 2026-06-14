package memory_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gotd/teled"
	"github.com/gotd/teled/memory"
)

func TestObjectStore(t *testing.T) {
	ctx := context.Background()
	s := memory.NewObjectStore()

	// Missing object.
	_, err := s.Get(ctx, "nope")
	require.Error(t, err)
	require.NoError(t, s.Delete(ctx, "nope")) // deleting missing is fine.

	const key = "abcd1234"

	require.NoError(t, s.Put(ctx, key, strings.NewReader("hello world"), 11, teled.PutOptions{}))

	info, err := s.Stat(ctx, key)
	require.NoError(t, err)
	require.Equal(t, int64(11), info.Size)

	rc, err := s.Get(ctx, key)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, "hello world", string(got))

	// Range read, including truncation past the end.
	rc, err = s.GetRange(ctx, key, 6, 5)
	require.NoError(t, err)

	got, _ = io.ReadAll(rc)
	require.Equal(t, "world", string(got))

	rc, err = s.GetRange(ctx, key, 6, 1000)
	require.NoError(t, err)

	got, _ = io.ReadAll(rc)
	require.Equal(t, "world", string(got))

	require.NoError(t, s.Delete(ctx, key))
	_, err = s.Get(ctx, key)
	require.Error(t, err)
}

func TestDBSatisfiesInterface(t *testing.T) {
	var _ teled.DB = memory.NewDB()
}

func TestDBUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	u, err := d.CreateUser(ctx, "+100", "Alice", "A")
	require.NoError(t, err)
	require.NotZero(t, u.ID)
	require.NotZero(t, u.AccessHash)

	// Phone uniqueness.
	_, err = d.CreateUser(ctx, "+100", "Dup", "")
	require.Error(t, err)

	got, ok, err := d.UserByPhone(ctx, "+100")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, u.ID, got.ID)

	_, ok, err = d.UserByID(ctx, 999)
	require.NoError(t, err)
	require.False(t, ok)

	var key [8]byte

	key[0] = 7
	require.NoError(t, d.BindSession(ctx, key, u.ID))
	id, ok, err := d.SessionUserID(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, u.ID, id)

	require.NoError(t, d.Unbind(ctx, key))
	_, ok, _ = d.SessionUserID(ctx, key)
	require.False(t, ok)
}

func TestDBBotFatherSeeded(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	bf, ok, err := d.UserByID(ctx, teled.BotFatherID)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, bf.IsBot)
	require.Equal(t, "BotFather", bf.Username)

	byName, ok, err := d.UserByUsername(ctx, "BotFather")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, teled.BotFatherID, byName.ID)
}

func TestDBOwnedBots(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	owner, err := d.CreateUser(ctx, "+1", "Owner", "")
	require.NoError(t, err)

	bot, err := d.CreateOwnedBot(ctx, "tetris_bot", "Tetris", owner.ID, "secret")
	require.NoError(t, err)
	require.True(t, bot.IsBot)
	require.Contains(t, bot.BotToken, ":secret")

	bots, err := d.BotsByOwner(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, bots, 1)
	require.Equal(t, bot.ID, bots[0].ID)

	found, ok, err := d.BotByToken(ctx, bot.BotToken)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, bot.ID, found.ID)

	// Revoke (replace token).
	require.NoError(t, d.SetBotToken(ctx, bot.ID, "newtoken"))
	_, ok, _ = d.BotByToken(ctx, bot.BotToken)
	require.False(t, ok)
	_, ok, _ = d.BotByToken(ctx, "newtoken")
	require.True(t, ok)
}

func TestDBMessagingFlow(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	alice, err := d.CreateUser(ctx, "+1", "Alice", "")
	require.NoError(t, err)
	bob, err := d.CreateUser(ctx, "+2", "Bob", "")
	require.NoError(t, err)

	sent, err := d.SendMessage(ctx, alice.ID, bob.ID, "hi", 42, 0)
	require.NoError(t, err)
	require.False(t, sent.SelfChat)
	require.Equal(t, 1, sent.SenderPts)
	require.Equal(t, int64(1), sent.SenderLocalID)
	require.Equal(t, int64(1), sent.RecipientLocalID)

	// Alice's outgoing view.
	hist, err := d.GetHistory(ctx, alice.ID, bob.ID, 0, 100)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	require.True(t, hist[0].Out)
	require.Equal(t, "hi", hist[0].Text)

	// Bob's incoming view.
	hist, err = d.GetHistory(ctx, bob.ID, alice.ID, 0, 100)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	require.False(t, hist[0].Out)
	require.Equal(t, alice.ID, hist[0].FromUserID)

	// Bob has one unread dialog.
	dialogs, err := d.GetDialogs(ctx, bob.ID, 100)
	require.NoError(t, err)
	require.Len(t, dialogs, 1)
	require.Equal(t, alice.ID, dialogs[0].PeerUserID)
	require.Equal(t, 1, dialogs[0].UnreadCount)

	// Bob reads history.
	_, err = d.ReadHistory(ctx, bob.ID, alice.ID, hist[0].LocalID)
	require.NoError(t, err)

	dialogs, _ = d.GetDialogs(ctx, bob.ID, 100)
	require.Equal(t, 0, dialogs[0].UnreadCount)
	require.Equal(t, hist[0].LocalID, dialogs[0].ReadInboxMaxID)

	// Alice edits her message.
	editRes, err := d.EditMessage(ctx, alice.ID, sent.SenderLocalID, "hello")
	require.NoError(t, err)
	require.Equal(t, bob.ID, editRes.PeerUserID)
	require.NotZero(t, editRes.PeerLocalID)

	hist, _ = d.GetHistory(ctx, bob.ID, alice.ID, 0, 100)
	require.Equal(t, "hello", hist[0].Text)
	require.False(t, hist[0].EditDate.IsZero())

	// Editing someone else's / missing message fails with ErrMessageID.
	_, err = d.EditMessage(ctx, bob.ID, 999, "x")
	require.ErrorIs(t, err, teled.ErrMessageID)

	// Alice deletes her message.
	delRes, err := d.DeleteMessages(ctx, alice.ID, []int64{sent.SenderLocalID})
	require.NoError(t, err)
	require.Equal(t, []int64{sent.SenderLocalID}, delRes.LocalIDs)

	hist, _ = d.GetHistory(ctx, alice.ID, bob.ID, 0, 100)
	require.Empty(t, hist)
}

func TestDBSelfChat(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()
	u, err := d.CreateUser(ctx, "+1", "Solo", "")
	require.NoError(t, err)

	sent, err := d.SendMessage(ctx, u.ID, u.ID, "note", 1, 0)
	require.NoError(t, err)
	require.True(t, sent.SelfChat)
	require.Zero(t, sent.RecipientLocalID)

	hist, err := d.GetHistory(ctx, u.ID, u.ID, 0, 100)
	require.NoError(t, err)
	require.Len(t, hist, 1)
}

func TestDBDifference(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()
	alice, _ := d.CreateUser(ctx, "+1", "Alice", "")
	bob, _ := d.CreateUser(ctx, "+2", "Bob", "")

	sent, err := d.SendMessage(ctx, alice.ID, bob.ID, "one", 1, 0)
	require.NoError(t, err)
	_, err = d.ReadHistory(ctx, bob.ID, alice.ID, 1)
	require.NoError(t, err)
	_, err = d.EditMessage(ctx, alice.ID, sent.SenderLocalID, "edited")
	require.NoError(t, err)
	_, err = d.DeleteMessages(ctx, alice.ID, []int64{sent.SenderLocalID})
	require.NoError(t, err)

	// Bob's log: "new", "readinbox" (his own read), then "edit" (Alice's edit
	// propagates to the peer). Delete only logs for the deleter, so it is absent.
	entries, current, err := d.GetDifference(ctx, bob.ID, 0, 100)
	require.NoError(t, err)
	require.NotZero(t, current)

	types := make([]string, len(entries))
	for i, e := range entries {
		types[i] = e.Type
		// pts is strictly increasing.
		if i > 0 {
			assert.Greater(t, e.Pts, entries[i-1].Pts)
		}
	}

	require.Equal(t, []string{teled.UpdateNew, teled.UpdateReadInbox, teled.UpdateEdit}, types)

	peer, maxID := teled.DecodeRead(entries[1].Extra)
	require.Equal(t, alice.ID, peer)
	require.Equal(t, int64(1), maxID)

	// Alice's log: new, readoutbox (Bob read her message), edit, delete.
	entries, _, err = d.GetDifference(ctx, alice.ID, 0, 100)
	require.NoError(t, err)

	types = types[:0]
	for _, e := range entries {
		types = append(types, e.Type)
	}

	require.Equal(t, []string{teled.UpdateNew, teled.UpdateReadOutbox, teled.UpdateEdit, teled.UpdateDelete}, types)

	rPeer, rMax := teled.DecodeRead(entries[1].Extra)
	require.Equal(t, bob.ID, rPeer) // the reader
	require.Equal(t, sent.SenderLocalID, rMax)
	require.Equal(t, []int64{sent.SenderLocalID}, teled.DecodeDeleted(entries[3].Extra))

	// sincePts filters.
	entries, _, err = d.GetDifference(ctx, alice.ID, entries[0].Pts, 100)
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestDBFilesAndMediaMessage(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()
	alice, _ := d.CreateUser(ctx, "+1", "Alice", "")
	bob, _ := d.CreateUser(ctx, "+2", "Bob", "")

	f, err := d.SaveFile(ctx, teled.File{OwnerUserID: alice.ID, ObjectKey: "k", Size: 3, Mime: "image/png"})
	require.NoError(t, err)
	require.NotZero(t, f.ID)
	require.NotZero(t, f.AccessHash)
	require.Len(t, f.FileReference, 16)
	require.Equal(t, "photo", f.Kind)

	got, ok, err := d.FileByID(ctx, f.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, f.ObjectKey, got.ObjectKey)

	sent, err := d.SendMessage(ctx, alice.ID, bob.ID, "look", 1, f.ID)
	require.NoError(t, err)
	hist, err := d.GetHistory(ctx, bob.ID, alice.ID, 0, 100)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	require.NotNil(t, hist[0].Media)
	require.Equal(t, f.ID, hist[0].Media.ID)

	_ = sent
}

func TestDBPhoneCodes(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	require.NoError(t, d.SavePhoneCode(ctx, "+1", "hash", "12345", time.Hour))
	code, ok, err := d.PhoneCode(ctx, "+1", "hash")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "12345", code)

	// Wrong hash.
	_, ok, _ = d.PhoneCode(ctx, "+1", "other")
	require.False(t, ok)

	// Expired.
	require.NoError(t, d.SavePhoneCode(ctx, "+1", "hash", "old", -time.Second))
	_, ok, _ = d.PhoneCode(ctx, "+1", "hash")
	require.False(t, ok)
}

func TestDBSearchUsers(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	_, err := d.CreateUser(ctx, "+1", "Alice", "Smith")
	require.NoError(t, err)

	// BotFather is seeded; it should be found by username prefix.
	found, err := d.SearchUsers(ctx, "botfa", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, teled.BotFatherID, found[0].ID)

	// Name substring, case-insensitive.
	found, err = d.SearchUsers(ctx, "ali", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "Alice", found[0].FirstName)

	// Empty query matches nothing.
	found, err = d.SearchUsers(ctx, "  ", 10)
	require.NoError(t, err)
	require.Empty(t, found)
}

func TestDBContacts(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	a, _ := d.CreateUser(ctx, "+1", "A", "")
	b, _ := d.CreateUser(ctx, "+2", "B", "")
	c, _ := d.CreateUser(ctx, "+3", "C", "")

	require.NoError(t, d.AddContact(ctx, a.ID, b.ID, "Bob", "B"))
	require.NoError(t, d.AddContact(ctx, a.ID, c.ID, "Cleo", "C"))

	got, err := d.Contacts(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "Bob", got[0].FirstName)

	require.NoError(t, d.DeleteContacts(ctx, a.ID, []int64{b.ID}))
	got, err = d.Contacts(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, c.ID, got[0].UserID)
}

func TestDBDrafts(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	a, _ := d.CreateUser(ctx, "+1", "A", "")
	b, _ := d.CreateUser(ctx, "+2", "B", "")

	_, err := d.SaveDraft(ctx, a.ID, b.ID, "hello draft")
	require.NoError(t, err)

	drafts, err := d.Drafts(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	require.Equal(t, "hello draft", drafts[0].Text)
	require.Equal(t, b.ID, drafts[0].PeerUserID)

	// Blank clears it.
	_, err = d.SaveDraft(ctx, a.ID, b.ID, "  ")
	require.NoError(t, err)
	drafts, err = d.Drafts(ctx, a.ID)
	require.NoError(t, err)
	require.Empty(t, drafts)
}

func TestDBTempAuthKeys(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()

	temp := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	perm := [8]byte{9, 10, 11, 12, 13, 14, 15, 16}

	// Unknown temp key resolves to nothing.
	_, ok, err := d.PermAuthKey(ctx, temp)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, d.BindTempAuthKey(ctx, temp, perm, time.Now().Add(time.Hour)))
	got, ok, err := d.PermAuthKey(ctx, temp)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, perm, got)

	// Expired binding is not returned.
	require.NoError(t, d.BindTempAuthKey(ctx, temp, perm, time.Now().Add(-time.Second)))
	_, ok, _ = d.PermAuthKey(ctx, temp)
	require.False(t, ok)
}

func TestDBBotFatherStateAndCommands(t *testing.T) {
	ctx := context.Background()
	d := memory.NewDB()
	u, _ := d.CreateUser(ctx, "+1", "U", "")

	st, err := d.BotFatherState(ctx, u.ID)
	require.NoError(t, err)
	require.Empty(t, st.Step)

	require.NoError(t, d.SetBotFatherState(ctx, u.ID, teled.BotFatherState{Step: teled.BotFatherStepNewBotName, DraftName: "T"}))
	st, _ = d.BotFatherState(ctx, u.ID)
	require.Equal(t, teled.BotFatherStepNewBotName, st.Step)
	require.Equal(t, "T", st.DraftName)

	require.NoError(t, d.ClearBotFatherState(ctx, u.ID))
	st, _ = d.BotFatherState(ctx, u.ID)
	require.Empty(t, st.Step)

	bot, _ := d.CreateBot(ctx, "tok", "b_bot", "B")
	// Missing row -> nil.
	cmds, err := d.BotCommands(ctx, bot.ID, "default", "")
	require.NoError(t, err)
	require.Nil(t, cmds)

	want := []teled.BotCommand{{Command: "start", Description: "Start"}}
	require.NoError(t, d.SetBotCommands(ctx, bot.ID, "default", "", want))
	cmds, _ = d.BotCommands(ctx, bot.ID, "default", "")
	require.Equal(t, want, cmds)

	// Empty (non-nil) list is stored as a present, empty row.
	require.NoError(t, d.SetBotCommands(ctx, bot.ID, "default", "", nil))
	cmds, _ = d.BotCommands(ctx, bot.ID, "default", "")
	require.NotNil(t, cmds)
	require.Empty(t, cmds)

	require.NoError(t, d.ResetBotCommands(ctx, bot.ID, "default", ""))
	cmds, _ = d.BotCommands(ctx, bot.ID, "default", "")
	require.Nil(t, cmds)
}
