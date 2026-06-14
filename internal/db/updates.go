package db

import (
	"context"
	"strconv"
	"time"

	gerrors "github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// GetState returns the caller's current update state.
func (db *DB) GetState(ctx context.Context, self int64) (teled.State, error) {
	var (
		st  teled.State
		pts int64
	)

	if err := db.pool.QueryRow(ctx, `SELECT pts FROM users WHERE id = $1`, self).Scan(&pts); err != nil {
		return teled.State{}, gerrors.Wrap(err, "scan")
	}

	st.Pts = int(pts)
	st.Date = time.Now()

	return st, nil
}

// MessageByGlobal returns the caller's view of a message by its canonical id.
func (db *DB) MessageByGlobal(ctx context.Context, self, globalID int64) (teled.Message, bool, error) {
	q := `
SELECT r.message_id, r.out, m.global_id, m.from_user_id, m.text, m.date, m.edit_date, m.random_id,
       CASE WHEN m.from_user_id = $1 THEN m.peer_user_id ELSE m.from_user_id END AS other,
       f.id, f.owner_user_id, f.access_hash, f.object_key, f.size, f.mime, f.sha256, f.file_reference, f.kind, f.created_at
FROM message_refs r
JOIN messages m ON m.global_id = r.global_id
LEFT JOIN files f ON f.id = m.media_file_id
WHERE r.user_id = $1 AND r.global_id = $2`

	rows, err := db.pool.Query(ctx, q, self, globalID)
	if err != nil {
		return teled.Message{}, false, gerrors.Wrap(err, "query")
	}

	defer rows.Close()

	if !rows.Next() {
		return teled.Message{}, false, rows.Err()
	}

	m, err := scanMessage(rows)
	if err != nil {
		return teled.Message{}, false, gerrors.Wrap(err, "scan")
	}

	return m, true, nil
}

// GetDifference returns log entries with pts greater than sincePts (up to
// limit) and the caller's current pts.
func (db *DB) GetDifference(ctx context.Context, self int64, sincePts, limit int) ([]teled.UpdateLogEntry, int, error) {
	current, err := db.GetState(ctx, self)
	if err != nil {
		return nil, 0, err
	}

	q := `SELECT pts, pts_count, type, global_id, extra, date FROM updates_log
	      WHERE user_id = $1 AND pts > $2 ORDER BY pts LIMIT ` + strconv.Itoa(limit)

	rows, err := db.pool.Query(ctx, q, self, sincePts)
	if err != nil {
		return nil, 0, gerrors.Wrap(err, "query")
	}

	defer rows.Close()

	var entries []teled.UpdateLogEntry

	for rows.Next() {
		var (
			e        teled.UpdateLogEntry
			globalID *int64
			extra    []byte
		)

		if err := rows.Scan(&e.Pts, &e.PtsCount, &e.Type, &globalID, &extra, &e.Date); err != nil {
			return nil, 0, gerrors.Wrap(err, "scan")
		}

		e.GlobalID = globalID
		e.Extra = extra
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, gerrors.Wrap(err, "rows")
	}

	return entries, current.Pts, nil
}
