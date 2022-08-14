package cmd

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-faster/errors"
	"github.com/go-faster/jx"
	"github.com/gotd/td/telegram/tljson"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgtest"

	"go.uber.org/zap"
)

// application holds application state.
type application struct {
	lg     *zap.Logger
	common *tgtest.Dispatcher

	Host           string
	Port           int
	PrivateKeyPath string
}

func (a application) Addr() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

func (a application) OnMessage(server *tgtest.Server, req *tgtest.Request) error {
	id, err := req.Buf.PeekID()
	if err != nil {
		return errors.Wrap(err, "peek id")
	}
	v, ok := tg.TypesMap()[id]
	if !ok {
		v = fmt.Sprintf("#%x", id)
	}

	a.lg.Fatal("Unexpected message",
		zap.String("type", v),
	)
	return nil
}

func (a application) OnAuthBindTempAuthKey(server *tgtest.Server, req *tgtest.Request) error {
	return server.SendBool(req, true)
}

func (a application) OnNearestDC(server *tgtest.Server, req *tgtest.Request) error {
	return server.SendResult(req, &tg.NearestDC{
		ThisDC:    1,
		NearestDC: 1,
		Country:   "Kekistan",
	})
}

func (a application) OnApplicationConfig(server *tgtest.Server, req *tgtest.Request) error {
	const data = `{
    "test": 1,
    "emojies_animated_zoom": 0.625,
    "emojies_send_dice": [
        "\ud83c\udfb2",
        "\ud83c\udfaf",
        "\ud83c\udfc0",
        "\u26bd",
        "\u26bd\ufe0f",
        "\ud83c\udfb0",
        "\ud83c\udfb3"
    ],
    "emojies_send_dice_success": {
        "\ud83c\udfaf": {
            "value": 6,
            "frame_start": 62
        },
        "\ud83c\udfc0": {
            "value": 5,
            "frame_start": 110
        },
        "\u26bd": {
            "value": 5,
            "frame_start": 110
        },
        "\u26bd\ufe0f": {
            "value": 5,
            "frame_start": 110
        },
        "\ud83c\udfb0": {
            "value": 64,
            "frame_start": 110
        },
        "\ud83c\udfb3": {
            "value": 6,
            "frame_start": 110
        }
    },
    "emojies_sounds": {
        "\ud83c\udf83": {
            "id": "4956223179606458539",
            "access_hash": "-2107001400913062971",
            "file_reference_base64": "AGFhvoKbftK5O9K9RpgN1ZtgSzWy"
        },
        "\u26b0": {
            "id": "4956223179606458540",
            "access_hash": "-1498869544183595185",
            "file_reference_base64": "AGFhvoJIm8Uz0qSMIdm3AsKlK7wJ"
        },
        "\ud83e\udddf\u200d\u2642": {
            "id": "4960929110848176331",
            "access_hash": "3986395821757915468",
            "file_reference_base64": "AGFhvoLtXSSIclmvfg6ePz3KsHQF"
        },
        "\ud83e\udddf": {
            "id": "4960929110848176332",
            "access_hash": "-8929417974289765626",
            "file_reference_base64": "AGFhvoImaz5Umt4GvMUD5nocIu0W"
        },
        "\ud83e\udddf\u200d\u2640": {
            "id": "4960929110848176333",
            "access_hash": "9161696144162881753",
            "file_reference_base64": "AGFhvoIm1QZsb48xlpRfh4Mq7EMG"
        },
        "\ud83c\udf51": {
            "id": "4963180910661861548",
            "access_hash": "-7431729439735063448",
            "file_reference_base64": "AGFhvoKLrwl_WKr5LR0Jjs7o3RyT"
        },
        "\ud83c\udf8a": {
            "id": "5094064004578410732",
            "access_hash": "8518192996098758509",
            "file_reference_base64": "AGFhvoKMNffRV2J3vKED0O6d8e42"
        },
        "\ud83c\udf84": {
            "id": "5094064004578410733",
            "access_hash": "-4142643820629256996",
            "file_reference_base64": "AGFhvoJ1ulPBbXEURlTZWwJFx6xZ"
        },
        "\ud83e\uddbe": {
            "id": "5094064004578410734",
            "access_hash": "-8934384022571962340",
            "file_reference_base64": "AGFhvoL4zdMRmYv9z3L8KPaX4JQL"
        }
    },
    "gif_search_branding": "tenor",
    "gif_search_emojies": [
        "\ud83d\udc4d",
        "\ud83d\ude18",
        "\ud83d\ude0d",
        "\ud83d\ude21",
        "\ud83e\udd73",
        "\ud83d\ude02",
        "\ud83d\ude2e",
        "\ud83d\ude44",
        "\ud83d\ude0e",
        "\ud83d\udc4e"
    ],
    "stickers_emoji_suggest_only_api": false,
    "stickers_emoji_cache_time": 86400,
    "qr_login_camera": false,
    "qr_login_code": "disabled",
    "dialog_filters_enabled": true,
    "dialog_filters_tooltip": false,
    "autoarchive_setting_available": false,
    "pending_suggestions": [
        "AUTOARCHIVE_POPULAR",
        "VALIDATE_PASSWORD",
        "VALIDATE_PHONE_NUMBER",
        "NEWCOMER_TICKS"
    ],
    "autologin_token": "string",
    "autologin_domains": [
        "instantview.telegram.org",
        "translations.telegram.org",
        "contest.dev",
        "contest.com",
        "bugs.telegram.org",
        "suggestions.telegram.org",
        "themes.telegram.org"
    ],
	"youtube_pip": "abc",
	"groupcall_video_participants_max": 1234,
    "url_auth_domains": [
        "somedomain.telegram.org"
    ],
    "round_video_encoding": {
        "diameter": 384,
        "video_bitrate": 1000,
        "audio_bitrate": 64,
        "max_size": 12582912
    },
    "chat_read_mark_size_threshold": 50,
    "chat_read_mark_expire_period": 604800,
	"unknown": null
}`
	d := jx.DecodeStr(data)
	v, err := tljson.Decode(d)
	if err != nil {
		return errors.Wrap(err, "decode")
	}
	return server.SendResult(req, v)
}

func (a application) OnCountriesList(server *tgtest.Server, req *tgtest.Request) error {
	return server.SendResult(req, &tg.HelpCountriesList{
		Hash: 1337,
		Countries: []tg.HelpCountry{
			{
				Iso2:        "AF",
				Name:        "Kekistan",
				DefaultName: "Kekistan",
				CountryCodes: []tg.HelpCountryCode{
					{
						CountryCode: "1337",
						Patterns: []string{
							"XXX XXX",
						},
						Prefixes: []string{
							"+1337",
							"1337",
						},
					},
				},
			},
		},
	})
}

func (a application) OnExportLoginToken(server *tgtest.Server, req *tgtest.Request) error {
	return server.SendResult(req, &tg.AuthLoginToken{
		Expires: int(time.Now().AddDate(0, 0, 1).Unix()),
		Token:   []byte("token"),
	})
}

func (a application) OnSendCode(server *tgtest.Server, req *tgtest.Request) error {
	return server.SendResult(req, &tg.AuthSentCode{
		PhoneCodeHash: "do-u-knw-da-way",
		Timeout:       1337,
	})
}
