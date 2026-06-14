package teled

// BotCommand is one entry in a bot's published command list.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// BotFatherID is the fixed user id of the built-in BotFather account. It
// mirrors real Telegram's BotFather id and is seeded by migration.
const BotFatherID int64 = 93372553

// BotFather conversation steps. An empty step means no flow is in progress.
const (
	BotFatherStepNewBotName     = "newbot_name"     // awaiting the new bot's display name
	BotFatherStepNewBotUsername = "newbot_username" // awaiting the new bot's username
	BotFatherStepRevokeSelect   = "revoke_select"   // awaiting which bot's token to revoke
)

// BotFatherState is a user's position in a multi-step BotFather flow.
type BotFatherState struct {
	Step      string
	DraftName string // the pending bot name captured during /newbot
}
