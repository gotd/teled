package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/bin"
)

// TestStartupStubsEncode ensures each canned startup response encodes without a
// nil-interface panic: several response structs have non-optional interface
// fields (e.g. AccountPassword.NewAlgo, MessagesWebPage.Webpage) that must be
// populated.
func TestStartupStubsEncode(t *testing.T) {
	ctx := context.Background()
	h := &Handler{}

	encoders := []func() (bin.Encoder, error){
		func() (bin.Encoder, error) { return h.messagesGetTopReactions(ctx, nil) },
		func() (bin.Encoder, error) { return h.messagesGetRecentReactions(ctx, nil) },
		func() (bin.Encoder, error) { return h.messagesGetDefaultTagReactions(ctx, 0) },
		func() (bin.Encoder, error) { return h.messagesGetSavedReactionTags(ctx, nil) },
		func() (bin.Encoder, error) { return h.accountGetReactionsNotifySettings(ctx) },
		func() (bin.Encoder, error) { return h.messagesGetAvailableEffects(ctx, 0) },
		func() (bin.Encoder, error) { return h.messagesGetEmojiGroups(ctx, 0) },
		func() (bin.Encoder, error) { return h.messagesGetEmojiStickerGroups(ctx, 0) },
		func() (bin.Encoder, error) { return h.accountGetDefaultEmojiStatuses(ctx, 0) },
		func() (bin.Encoder, error) { return h.accountGetCollectibleEmojiStatuses(ctx, 0) },
		func() (bin.Encoder, error) { return h.messagesGetQuickReplies(ctx, 0) },
		func() (bin.Encoder, error) { return h.aicomposeGetTones(ctx, 0) },
		func() (bin.Encoder, error) { return h.paymentsGetStarGiftActiveAuctions(ctx, 0) },
		func() (bin.Encoder, error) { return h.helpGetPeerColors(ctx, 0) },
		func() (bin.Encoder, error) { return h.helpGetPeerProfileColors(ctx, 0) },
		func() (bin.Encoder, error) { return h.accountGetThemes(ctx, nil) },
		func() (bin.Encoder, error) { return h.storiesGetAllStories(ctx, nil) },
		func() (bin.Encoder, error) { return h.messagesGetPinnedSavedDialogs(ctx) },
		func() (bin.Encoder, error) { return h.messagesGetPeerSettings(ctx, nil) },
		func() (bin.Encoder, error) { return h.messagesGetScheduledHistory(ctx, nil) },
		func() (bin.Encoder, error) { return h.messagesGetWebPage(ctx, nil) },
		func() (bin.Encoder, error) { return h.contactsGetTopPeers(ctx, nil) },
		func() (bin.Encoder, error) { return h.accountGetPassword(ctx) },
		func() (bin.Encoder, error) { return h.accountGetContentSettings(ctx) },
	}

	for i, fn := range encoders {
		res, err := fn()
		require.NoErrorf(t, err, "handler %d returned error", i)

		var b bin.Buffer

		require.NoErrorf(t, res.Encode(&b), "handler %d failed to encode", i)
		require.NotZerof(t, b.Len(), "handler %d encoded to empty buffer", i)
	}
}
