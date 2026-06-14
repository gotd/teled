package rpc

import (
	"context"

	"github.com/gotd/td/tg"
)

// This file holds canned responses for the many read-only RPCs a full client
// (notably Telegram Desktop) fires on startup to populate reactions, emoji,
// stories, themes, colors, drafts and similar feature state. teled implements
// none of these features, so each returns an empty or "not modified" result:
// enough for the client to initialize cleanly instead of treating the account
// as broken on a stream of NOT_IMPLEMENTED errors.

// Reactions.

func (h *Handler) messagesGetTopReactions(context.Context, *tg.MessagesGetTopReactionsRequest) (tg.MessagesReactionsClass, error) {
	return &tg.MessagesReactionsNotModified{}, nil
}

func (h *Handler) messagesGetRecentReactions(context.Context, *tg.MessagesGetRecentReactionsRequest) (tg.MessagesReactionsClass, error) {
	return &tg.MessagesReactionsNotModified{}, nil
}

func (h *Handler) messagesGetDefaultTagReactions(context.Context, int64) (tg.MessagesReactionsClass, error) {
	return &tg.MessagesReactionsNotModified{}, nil
}

func (h *Handler) messagesGetSavedReactionTags(
	context.Context, *tg.MessagesGetSavedReactionTagsRequest,
) (tg.MessagesSavedReactionTagsClass, error) {
	return &tg.MessagesSavedReactionTagsNotModified{}, nil
}

func (h *Handler) accountGetReactionsNotifySettings(context.Context) (*tg.ReactionsNotifySettings, error) {
	return &tg.ReactionsNotifySettings{Sound: &tg.NotificationSoundDefault{}}, nil
}

// Emoji / effects.

func (h *Handler) messagesGetAvailableEffects(context.Context, int) (tg.MessagesAvailableEffectsClass, error) {
	return &tg.MessagesAvailableEffects{}, nil
}

func (h *Handler) messagesGetEmojiGroups(context.Context, int) (tg.MessagesEmojiGroupsClass, error) {
	return &tg.MessagesEmojiGroupsNotModified{}, nil
}

func (h *Handler) messagesGetEmojiStickerGroups(context.Context, int) (tg.MessagesEmojiGroupsClass, error) {
	return &tg.MessagesEmojiGroupsNotModified{}, nil
}

func (h *Handler) messagesGetEmojiKeywordsLanguages(context.Context, []string) ([]tg.EmojiLanguage, error) {
	return nil, nil
}

func (h *Handler) accountGetDefaultEmojiStatuses(context.Context, int64) (tg.AccountEmojiStatusesClass, error) {
	return &tg.AccountEmojiStatusesNotModified{}, nil
}

func (h *Handler) accountGetCollectibleEmojiStatuses(context.Context, int64) (tg.AccountEmojiStatusesClass, error) {
	return &tg.AccountEmojiStatusesNotModified{}, nil
}

// Quick replies, effects, AI compose, gifts.

func (h *Handler) messagesGetQuickReplies(context.Context, int64) (tg.MessagesQuickRepliesClass, error) {
	return &tg.MessagesQuickReplies{}, nil
}

func (h *Handler) aicomposeGetTones(context.Context, int64) (tg.AicomposeTonesClass, error) {
	return &tg.AicomposeTones{}, nil
}

func (h *Handler) paymentsGetStarGiftActiveAuctions(context.Context, int64) (tg.PaymentsStarGiftActiveAuctionsClass, error) {
	return &tg.PaymentsStarGiftActiveAuctions{}, nil
}

// Colors / themes.

func (h *Handler) helpGetPeerColors(context.Context, int) (tg.HelpPeerColorsClass, error) {
	return &tg.HelpPeerColorsNotModified{}, nil
}

func (h *Handler) helpGetPeerProfileColors(context.Context, int) (tg.HelpPeerColorsClass, error) {
	return &tg.HelpPeerColorsNotModified{}, nil
}

func (h *Handler) accountGetThemes(context.Context, *tg.AccountGetThemesRequest) (tg.AccountThemesClass, error) {
	return &tg.AccountThemesNotModified{}, nil
}

// Stories.

func (h *Handler) storiesGetAllStories(context.Context, *tg.StoriesGetAllStoriesRequest) (tg.StoriesAllStoriesClass, error) {
	// Return a complete (empty) story list rather than "not modified": a client
	// with no cached state that receives NotModified keeps polling forever.
	return &tg.StoriesAllStories{State: "0"}, nil
}

func (h *Handler) storiesGetStoriesArchive(context.Context, *tg.StoriesGetStoriesArchiveRequest) (*tg.StoriesStories, error) {
	return &tg.StoriesStories{}, nil
}

func (h *Handler) storiesGetPinnedStories(context.Context, *tg.StoriesGetPinnedStoriesRequest) (*tg.StoriesStories, error) {
	return &tg.StoriesStories{}, nil
}

// Dialogs / drafts / search / peers.

func (h *Handler) messagesGetPinnedSavedDialogs(context.Context) (tg.MessagesSavedDialogsClass, error) {
	return &tg.MessagesSavedDialogs{}, nil
}

func (h *Handler) messagesGetSuggestedDialogFilters(context.Context) ([]tg.DialogFilterSuggested, error) {
	return nil, nil
}

func (h *Handler) messagesGetPeerSettings(context.Context, tg.InputPeerClass) (*tg.MessagesPeerSettings, error) {
	return &tg.MessagesPeerSettings{}, nil
}

func (h *Handler) messagesGetScheduledHistory(context.Context, *tg.MessagesGetScheduledHistoryRequest) (tg.MessagesMessagesClass, error) {
	return &tg.MessagesMessages{}, nil
}

func (h *Handler) messagesGetWebPage(context.Context, *tg.MessagesGetWebPageRequest) (*tg.MessagesWebPage, error) {
	return &tg.MessagesWebPage{Webpage: &tg.WebPageEmpty{}}, nil
}

func (h *Handler) messagesGetWebPagePreview(context.Context, *tg.MessagesGetWebPagePreviewRequest) (*tg.MessagesWebPagePreview, error) {
	return &tg.MessagesWebPagePreview{Media: &tg.MessageMediaEmpty{}}, nil
}

func (h *Handler) contactsGetTopPeers(context.Context, *tg.ContactsGetTopPeersRequest) (tg.ContactsTopPeersClass, error) {
	return &tg.ContactsTopPeersNotModified{}, nil
}

// Account state.

func (h *Handler) accountGetPassword(context.Context) (*tg.AccountPassword, error) {
	// No 2FA configured: report no password, with placeholder KDF algorithms so
	// the struct's non-optional fields are populated.
	return &tg.AccountPassword{
		NewAlgo:       &tg.PasswordKdfAlgoUnknown{},
		NewSecureAlgo: &tg.SecurePasswordKdfAlgoUnknown{},
		SecureRandom:  []byte{},
	}, nil
}

func (h *Handler) accountGetContentSettings(context.Context) (*tg.AccountContentSettings, error) {
	return &tg.AccountContentSettings{}, nil
}

func (h *Handler) accountGetContactSignUpNotification(context.Context) (bool, error) {
	return true, nil
}
