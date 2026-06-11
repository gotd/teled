package rpc

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/go-faster/jx"

	"github.com/gotd/td/telegram/tljson"
	"github.com/gotd/td/tg"
)

const countryName = "Kekistan"

func (h *Handler) helpGetConfig(context.Context) (*tg.Config, error) {
	return &tg.Config{
		Date:    int(time.Now().Unix()),
		Expires: int(time.Now().AddDate(0, 0, 1).Unix()),
		ThisDC:  h.dc,
		DCOptions: []tg.DCOption{
			{
				ID:           h.dc,
				IPAddress:    h.host,
				Port:         h.port,
				ThisPortOnly: true,
			},
		},
	}, nil
}

func (h *Handler) helpGetNearestDC(context.Context) (*tg.NearestDC, error) {
	return &tg.NearestDC{
		ThisDC:    h.dc,
		NearestDC: h.dc,
		Country:   countryName,
	}, nil
}

func (h *Handler) helpGetAppConfig(context.Context, int) (tg.HelpAppConfigClass, error) {
	v, err := tljson.Decode(jx.DecodeStr(defaultAppConfig))
	if err != nil {
		return nil, errors.Wrap(err, "decode")
	}
	return &tg.HelpAppConfig{Config: v}, nil
}

func (h *Handler) helpGetCountriesList(context.Context, *tg.HelpGetCountriesListRequest) (tg.HelpCountriesListClass, error) {
	return &tg.HelpCountriesList{
		Hash: 1337,
		Countries: []tg.HelpCountry{
			{
				ISO2:        "AF",
				Name:        countryName,
				DefaultName: countryName,
				CountryCodes: []tg.HelpCountryCode{
					{CountryCode: "1337", Patterns: []string{"XXX XXX"}, Prefixes: []string{"+1337", "1337"}},
				},
			},
		},
	}, nil
}

func (h *Handler) helpGetTermsOfServiceUpdate(context.Context) (tg.HelpTermsOfServiceUpdateClass, error) {
	return &tg.HelpTermsOfServiceUpdateEmpty{
		Expires: int(time.Now().AddDate(0, 0, 1).Unix()),
	}, nil
}

func (h *Handler) helpGetPremiumPromo(context.Context) (*tg.HelpPremiumPromo, error) {
	return &tg.HelpPremiumPromo{StatusText: "MORE GOLD"}, nil
}

func (h *Handler) helpGetPromoData(context.Context) (tg.HelpPromoDataClass, error) {
	return &tg.HelpPromoDataEmpty{}, nil
}

func (h *Handler) helpDismissSuggestion(context.Context, *tg.HelpDismissSuggestionRequest) (bool, error) {
	return true, nil
}

func (h *Handler) helpAcceptTermsOfService(context.Context, tg.DataJSON) (bool, error) {
	return true, nil
}

func (h *Handler) authExportLoginToken(context.Context, *tg.AuthExportLoginTokenRequest) (tg.AuthLoginTokenClass, error) {
	return &tg.AuthLoginToken{
		Expires: int(time.Now().AddDate(0, 0, 1).Unix()),
		Token:   []byte("token"),
	}, nil
}

func (h *Handler) authBindTempAuthKey(context.Context, *tg.AuthBindTempAuthKeyRequest) (bool, error) {
	return true, nil
}

func (h *Handler) contactsGetContacts(context.Context, int64) (tg.ContactsContactsClass, error) {
	return &tg.ContactsContacts{}, nil
}

func (h *Handler) accountGetNotifySettings(context.Context, tg.InputNotifyPeerClass) (*tg.PeerNotifySettings, error) {
	return &tg.PeerNotifySettings{}, nil
}

func (h *Handler) accountGetGlobalPrivacySettings(context.Context) (*tg.GlobalPrivacySettings, error) {
	return &tg.GlobalPrivacySettings{}, nil
}

func (h *Handler) messagesGetDialogFilters(context.Context) (*tg.MessagesDialogFilters, error) {
	return &tg.MessagesDialogFilters{Filters: []tg.DialogFilterClass{&tg.DialogFilterDefault{}}}, nil
}

func (h *Handler) messagesGetAvailableReactions(context.Context, int) (tg.MessagesAvailableReactionsClass, error) {
	return &tg.MessagesAvailableReactions{Hash: 1337, Reactions: []tg.AvailableReaction{}}, nil
}

func (h *Handler) messagesGetStickerSet(context.Context, *tg.MessagesGetStickerSetRequest) (tg.MessagesStickerSetClass, error) {
	return &tg.MessagesStickerSet{}, nil
}

func (h *Handler) messagesGetAttachMenuBots(context.Context, int64) (tg.AttachMenuBotsClass, error) {
	return &tg.AttachMenuBots{}, nil
}

func (h *Handler) messagesGetPinnedDialogs(context.Context, int) (*tg.MessagesPeerDialogs, error) {
	return &tg.MessagesPeerDialogs{}, nil
}

func (h *Handler) messagesGetStickers(context.Context, *tg.MessagesGetStickersRequest) (tg.MessagesStickersClass, error) {
	return &tg.MessagesStickers{Hash: 1337}, nil
}

func (h *Handler) messagesGetPeerDialogs(context.Context, []tg.InputDialogPeerClass) (*tg.MessagesPeerDialogs, error) {
	return &tg.MessagesPeerDialogs{}, nil
}

func (h *Handler) channelsGetMessages(context.Context, *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	return &tg.MessagesChannelMessages{}, nil
}
