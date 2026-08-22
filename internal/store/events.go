package store

import (
	"context"
	"database/sql"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Events
// --------------------------------------------------------------------------- //

func (q *Queries) AppendEvent(ctx context.Context, e model.Event) error {
	id := e.ID
	if id == "" {
		id = nextID()
	}
	_, err := q.db.ExecContext(ctx, `
INSERT INTO events (id,type,satellite_id,payload_json,ts_sec,created_at)
VALUES (?,?,?,?,?,?)`, id, string(e.Type), nullableString(e.SatelliteID), e.PayloadJSON, int64(e.Ts), int64(e.CreatedAt))
	return err
}

// ListEventsOrdered returns all events ordered for replay. The primary key is
// ts_sec ascending; the tie-breaker for events recorded in the same second is
// created_at, and the final tie-breaker is id ascending. Several service flows
// (RegisterSatellite, PushElements, ForecastContacts, ...) insert multiple
// events in one transaction with an identical clock reading, so created_at
// alone does not disambiguate them. The id is assigned monotonically from
// idlib.Generator at insert time, so id order reproduces the true recording
// order and yields a stable, restart-independent replay sequence.
func (q *Queries) ListEventsOrdered(ctx context.Context) ([]model.Event, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT id,type,satellite_id,payload_json,ts_sec,created_at FROM events
ORDER BY ts_sec ASC, created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Event
	for rows.Next() {
		var e model.Event
		var typ string
		var sat sql.NullString
		var ts, ca int64
		if err := rows.Scan(&e.ID, &typ, &sat, &e.PayloadJSON, &ts, &ca); err != nil {
			return nil, err
		}
		e.Type = model.EventType(typ)
		if sat.Valid {
			e.SatelliteID = sat.String
		}
		e.Ts = model.Epoch(ts)
		e.CreatedAt = model.Epoch(ca)
		out = append(out, e)
	}
	return out, rows.Err()
}

// MaxEventTs returns the highest ts_sec in the events table, or 0 if empty.
func (q *Queries) MaxEventTs(ctx context.Context) (model.Epoch, error) {
	row := q.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(ts_sec),0) FROM events`)
	var ts int64
	if err := row.Scan(&ts); err != nil {
		return 0, err
	}
	return model.Epoch(ts), nil
}
