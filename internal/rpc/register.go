package rpc

import "github.com/gotd/td/tg"

// register wires all supported RPCs onto the dispatcher.
func (h *Handler) register(d *tg.ServerDispatcher) {
	// Help / config.
	d.OnHelpGetConfig(h.helpGetConfig)
	d.OnHelpGetNearestDC(h.helpGetNearestDC)
	d.OnHelpGetAppConfig(h.helpGetAppConfig)
	d.OnHelpGetCountriesList(h.helpGetCountriesList)
	d.OnHelpGetTermsOfServiceUpdate(h.helpGetTermsOfServiceUpdate)
	d.OnHelpGetPremiumPromo(h.helpGetPremiumPromo)
	d.OnHelpGetPromoData(h.helpGetPromoData)
	d.OnHelpDismissSuggestion(h.helpDismissSuggestion)
	d.OnHelpAcceptTermsOfService(h.helpAcceptTermsOfService)

	// Auth (M2, storage-backed).
	d.OnAuthSendCode(h.authSendCode)
	d.OnAuthSignIn(h.authSignIn)
	d.OnAuthSignUp(h.authSignUp)
	d.OnAuthLogOut(h.authLogOut)
	d.OnAuthExportLoginToken(h.authExportLoginToken)
	d.OnAuthBindTempAuthKey(h.authBindTempAuthKey)
	d.OnAuthImportBotAuthorization(h.authImportBotAuthorization)

	// Bots.
	d.OnBotsSetBotCommands(h.botsSetBotCommands)
	d.OnBotsGetBotCommands(h.botsGetBotCommands)
	d.OnBotsResetBotCommands(h.botsResetBotCommands)

	// Users (M2, storage-backed).
	d.OnUsersGetUsers(h.usersGetUsers)
	d.OnUsersGetFullUser(h.usersGetFullUser)

	// Contacts.
	d.OnContactsResolveUsername(h.contactsResolveUsername)
	d.OnContactsGetContacts(h.contactsGetContacts)

	// Account.
	d.OnAccountGetNotifySettings(h.accountGetNotifySettings)
	d.OnAccountGetGlobalPrivacySettings(h.accountGetGlobalPrivacySettings)
	d.OnAccountGetPrivacy(h.accountGetPrivacy)
	d.OnAccountGetConnectedBots(h.accountGetConnectedBots)
	d.OnAccountCheckUsername(h.accountCheckUsername)
	d.OnAccountUpdateUsername(h.accountUpdateUsername)

	// Messages.
	d.OnMessagesSendMessage(h.messagesSendMessage)
	d.OnMessagesGetHistory(h.messagesGetHistory)
	d.OnMessagesGetDialogs(h.messagesGetDialogs)
	d.OnMessagesGetSavedHistory(h.messagesGetSavedHistory)
	d.OnMessagesReadHistory(h.messagesReadHistory)
	d.OnMessagesEditMessage(h.messagesEditMessage)
	d.OnMessagesDeleteMessages(h.messagesDeleteMessages)
	d.OnMessagesGetDialogFilters(h.messagesGetDialogFilters)
	d.OnMessagesGetAvailableReactions(h.messagesGetAvailableReactions)
	d.OnMessagesGetStickerSet(h.messagesGetStickerSet)
	d.OnMessagesGetAttachMenuBots(h.messagesGetAttachMenuBots)
	d.OnMessagesGetDialogs(h.messagesGetDialogs)
	d.OnMessagesGetPinnedDialogs(h.messagesGetPinnedDialogs)
	d.OnMessagesGetStickers(h.messagesGetStickers)
	d.OnMessagesGetAllStickers(h.messagesGetAllStickers)
	d.OnMessagesGetEmojiStickers(h.messagesGetEmojiStickers)
	d.OnMessagesGetFavedStickers(h.messagesGetFavedStickers)
	d.OnMessagesGetFeaturedStickers(h.messagesGetFeaturedStickers)
	d.OnMessagesGetFeaturedEmojiStickers(h.messagesGetFeaturedEmojiStickers)
	d.OnMessagesGetPeerDialogs(h.messagesGetPeerDialogs)

	// Updates (stubs until M4).
	d.OnUpdatesGetState(h.updatesGetState)
	d.OnUpdatesGetDifference(h.updatesGetDifference)

	// Media.
	d.OnUploadSaveFilePart(h.uploadSaveFilePart)
	d.OnUploadSaveBigFilePart(h.uploadSaveBigFilePart)
	d.OnMessagesSendMedia(h.messagesSendMedia)
	d.OnUploadGetFile(h.uploadGetFile)

	// Channels (out of scope; stub).
	d.OnChannelsGetMessages(h.channelsGetMessages)

	// Startup feature probes (read-only stubs; see startup_stubs.go).
	d.OnMessagesGetTopReactions(h.messagesGetTopReactions)
	d.OnMessagesGetRecentReactions(h.messagesGetRecentReactions)
	d.OnMessagesGetDefaultTagReactions(h.messagesGetDefaultTagReactions)
	d.OnMessagesGetSavedReactionTags(h.messagesGetSavedReactionTags)
	d.OnAccountGetReactionsNotifySettings(h.accountGetReactionsNotifySettings)
	d.OnMessagesGetAvailableEffects(h.messagesGetAvailableEffects)
	d.OnMessagesGetEmojiGroups(h.messagesGetEmojiGroups)
	d.OnMessagesGetEmojiStickerGroups(h.messagesGetEmojiStickerGroups)
	d.OnMessagesGetEmojiKeywordsLanguages(h.messagesGetEmojiKeywordsLanguages)
	d.OnAccountGetDefaultEmojiStatuses(h.accountGetDefaultEmojiStatuses)
	d.OnAccountGetCollectibleEmojiStatuses(h.accountGetCollectibleEmojiStatuses)
	d.OnMessagesGetQuickReplies(h.messagesGetQuickReplies)
	d.OnAicomposeGetTones(h.aicomposeGetTones)
	d.OnPaymentsGetStarGiftActiveAuctions(h.paymentsGetStarGiftActiveAuctions)
	d.OnHelpGetPeerColors(h.helpGetPeerColors)
	d.OnHelpGetPeerProfileColors(h.helpGetPeerProfileColors)
	d.OnAccountGetThemes(h.accountGetThemes)
	d.OnStoriesGetAllStories(h.storiesGetAllStories)
	d.OnStoriesGetStoriesArchive(h.storiesGetStoriesArchive)
	d.OnMessagesGetPinnedSavedDialogs(h.messagesGetPinnedSavedDialogs)
	d.OnMessagesGetSuggestedDialogFilters(h.messagesGetSuggestedDialogFilters)
	d.OnMessagesSaveDraft(h.messagesSaveDraft)
	d.OnMessagesGetPeerSettings(h.messagesGetPeerSettings)
	d.OnMessagesSearch(h.messagesSearch)
	d.OnMessagesGetScheduledHistory(h.messagesGetScheduledHistory)
	d.OnMessagesGetWebPage(h.messagesGetWebPage)
	d.OnMessagesGetWebPagePreview(h.messagesGetWebPagePreview)
	d.OnContactsGetTopPeers(h.contactsGetTopPeers)
	d.OnAccountGetPassword(h.accountGetPassword)
	d.OnAccountGetContentSettings(h.accountGetContentSettings)
	d.OnAccountGetContactSignUpNotification(h.accountGetContactSignUpNotification)
	d.OnAccountUpdateProfile(h.accountUpdateProfile)
}
