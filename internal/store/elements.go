package store

import (
	"context"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Element history
// --------------------------------------------------------------------------- //

func (q *Queries) InsertElementHistory(ctx context.Context, h model.ElementHistory) error {
	id := h.ID
	if id == "" {
		id = nextID()
	}
	_, err := q.db.ExecContext(ctx, `
INSERT INTO element_history (id,satellite_id,a,e,i,raan,argp,m,epoch_sec,source,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, id, h.SatelliteID, h.Elements.A, h.Elements.E, h.Elements.I,
		h.Elements.Raan, h.Elements.Argp, h.Elements.M, int64(h.Elements.Epoch), string(h.Source), int64(h.CreatedAt))
	return err
}

// ListElementHistory returns a satellite's element-history rows in epoch order.
// The tie-breakers (created_at, then id ascending) keep the order stable and
// restart-independent for rows recorded in the same second (e.g. the initial
// history row and a tle_update written in one transaction share an identical
// clock reading). id is assigned monotonically at insert time, so it encodes
// the true recording order when created_at cannot disambiguate.
func (q *Queries) ListElementHistory(ctx context.Context, satID string) ([]model.ElementHistory, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT id,satellite_id,a,e,i,raan,argp,m,epoch_sec,source,created_at
FROM element_history WHERE satellite_id=? ORDER BY epoch_sec ASC, created_at ASC, id ASC`, satID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ElementHistory
	for rows.Next() {
		var h model.ElementHistory
		var src string
		var ep, ca int64
		if err := rows.Scan(&h.ID, &h.SatelliteID, &h.Elements.A, &h.Elements.E, &h.Elements.I,
			&h.Elements.Raan, &h.Elements.Argp, &h.Elements.M, &ep, &src, &ca); err != nil {
			return nil, err
		}
		h.Elements.Epoch = model.Epoch(ep)
		h.Source = model.ElementSource(src)
		h.CreatedAt = model.Epoch(ca)
		out = append(out, h)
	}
	return out, rows.Err()
}

// LatestElementHistory returns the last element-history row for a satellite
// (highest epoch, then highest created_at, then highest id). The id
// tie-breaker makes the selection deterministic when several rows share one
// clock reading; it reflects the most recently inserted row at that instant.
// Used as the reconciliation baseline.
func (q *Queries) LatestElementHistory(ctx context.Context, satID string) (model.ElementHistory, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,satellite_id,a,e,i,raan,argp,m,epoch_sec,source,created_at
FROM element_history WHERE satellite_id=? ORDER BY epoch_sec DESC, created_at DESC, id DESC LIMIT 1`, satID)
	var h model.ElementHistory
	var src string
	var ep, ca int64
	err := row.Scan(&h.ID, &h.SatelliteID, &h.Elements.A, &h.Elements.E, &h.Elements.I,
		&h.Elements.Raan, &h.Elements.Argp, &h.Elements.M, &ep, &src, &ca)
	if err != nil {
		if errNoRows(err) {
			return model.ElementHistory{}, model.ErrSatelliteNotFound
		}
		return model.ElementHistory{}, err
	}
	h.Elements.Epoch = model.Epoch(ep)
	h.Source = model.ElementSource(src)
	h.CreatedAt = model.Epoch(ca)
	return h, nil
}
