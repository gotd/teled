package teled

// BotCommand is one entry in a bot's published command list.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}
