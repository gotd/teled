package teled

import "time"

// User is a teled account.
type User struct {
	ID         int64
	AccessHash int64
	Phone      string
	Username   string
	FirstName  string
	LastName   string
	About      string
	IsBot      bool   // true for bot accounts authenticated by token
	BotToken   string // bot auth token, empty for human accounts
	CreatedAt  time.Time
}
