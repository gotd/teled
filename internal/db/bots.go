package db

import (
	"context"
	"encoding/json"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// SetBotCommands replaces the command list a bot publishes for the given scope
// and language. An empty commands slice stores an empty list (distinct from
// having no row, which ResetBotCommands produces).
func (db *DB) SetBotCommands(ctx context.Context, botUserID int64, scope, langCode string, commands []teled.BotCommand) error {
	if commands == nil {
		commands = []teled.BotCommand{}
	}
	payload, err := json.Marshal(commands)
	if err != nil {
		return gerrors.Wrap(err, "marshal commands")
	}

	q := psql.Insert("bot_commands").
		Columns("bot_user_id", "scope", "lang_code", "commands").
		Values(botUserID, scope, langCode, payload).
		Suffix("ON CONFLICT (bot_user_id, scope, lang_code) DO UPDATE SET commands = EXCLUDED.commands")

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}
	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}
	return nil
}

// BotCommands returns the command list published for the given scope and
// language. A missing row yields an empty slice and no error.
func (db *DB) BotCommands(ctx context.Context, botUserID int64, scope, langCode string) ([]teled.BotCommand, error) {
	q := psql.Select("commands").From("bot_commands").
		Where("bot_user_id = ? AND scope = ? AND lang_code = ?", botUserID, scope, langCode)

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, gerrors.Wrap(err, "build query")
	}

	var payload []byte
	if err := db.pool.QueryRow(ctx, sql, args...).Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, gerrors.Wrap(err, "scan")
	}

	var commands []teled.BotCommand
	if err := json.Unmarshal(payload, &commands); err != nil {
		return nil, gerrors.Wrap(err, "unmarshal commands")
	}
	return commands, nil
}

// ResetBotCommands removes the command list for the given scope and language,
// restoring the bot's default (empty) commands there.
func (db *DB) ResetBotCommands(ctx context.Context, botUserID int64, scope, langCode string) error {
	q := psql.Delete("bot_commands").
		Where("bot_user_id = ? AND scope = ? AND lang_code = ?", botUserID, scope, langCode)

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}
	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}
	return nil
}
