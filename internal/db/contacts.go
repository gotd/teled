package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	gerrors "github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// AddContact saves (or updates) contactID in ownerID's contact list.
func (db *DB) AddContact(ctx context.Context, ownerID, contactID int64, firstName, lastName string) error {
	q := psql.Insert("contacts").
		Columns("owner_user_id", "contact_user_id", "first_name", "last_name").
		Values(ownerID, contactID, firstName, lastName).
		Suffix("ON CONFLICT (owner_user_id, contact_user_id) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name")

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}

	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}

	return nil
}

// DeleteContacts removes the given users from ownerID's contact list.
func (db *DB) DeleteContacts(ctx context.Context, ownerID int64, contactIDs []int64) error {
	if len(contactIDs) == 0 {
		return nil
	}

	q := psql.Delete("contacts").Where(sq.Eq{"owner_user_id": ownerID, "contact_user_id": contactIDs})

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}

	if _, err := db.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}

	return nil
}

// Contacts returns ownerID's saved contacts, ordered by user id.
func (db *DB) Contacts(ctx context.Context, ownerID int64) ([]teled.Contact, error) {
	q := psql.Select("contact_user_id", "first_name", "last_name").From("contacts").
		Where("owner_user_id = ?", ownerID).OrderBy("contact_user_id")

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, gerrors.Wrap(err, "build query")
	}

	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, gerrors.Wrap(err, "query")
	}
	defer rows.Close()

	var contacts []teled.Contact

	for rows.Next() {
		var c teled.Contact
		if err := rows.Scan(&c.UserID, &c.FirstName, &c.LastName); err != nil {
			return nil, gerrors.Wrap(err, "scan")
		}

		contacts = append(contacts, c)
	}

	if err := rows.Err(); err != nil {
		return nil, gerrors.Wrap(err, "rows")
	}

	return contacts, nil
}
