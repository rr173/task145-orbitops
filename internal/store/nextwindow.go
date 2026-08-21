package store

import (
	"context"
	"database/sql"
	"errors"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Next-window cache
// --------------------------------------------------------------------------- //

func (q *Queries) UpsertNextWindow(ctx context.Context, nw model.NextWindow) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO next_window_cache (satellite_id,next_contact_epoch,next_contact_station,next_maneuver_epoch,next_maneuver_type,open_alerts)
VALUES (?,?,?,?,?,?)
ON CONFLICT(satellite_id) DO UPDATE SET
  next_contact_epoch=excluded.next_contact_epoch,
  next_contact_station=excluded.next_contact_station,
  next_maneuver_epoch=excluded.next_maneuver_epoch,
  next_maneuver_type=excluded.next_maneuver_type,
  open_alerts=excluded.open_alerts`, nw.SatelliteID, int64(nw.NextContactEpoch), nw.NextContactStation,
		int64(nw.NextManeuverEpoch), string(nw.NextManeuverType), nw.OpenAlerts)
	return err
}

func (q *Queries) GetNextWindow(ctx context.Context, satID string) (model.NextWindow, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT satellite_id,next_contact_epoch,next_contact_station,next_maneuver_epoch,next_maneuver_type,open_alerts
FROM next_window_cache WHERE satellite_id=?`, satID)
	var nw model.NextWindow
	var typ string
	var nce, nme int64
	var ncs string
	err := row.Scan(&nw.SatelliteID, &nce, &ncs, &nme, &typ, &nw.OpenAlerts)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.NextWindow{}, model.ErrSatelliteNotFound
		}
		return model.NextWindow{}, err
	}
	nw.NextContactEpoch = model.Epoch(nce)
	nw.NextContactStation = ncs
	nw.NextManeuverEpoch = model.Epoch(nme)
	nw.NextManeuverType = model.ManeuverType(typ)
	return nw, nil
}

func (q *Queries) ListNextWindows(ctx context.Context) ([]model.NextWindow, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT satellite_id,next_contact_epoch,next_contact_station,next_maneuver_epoch,next_maneuver_type,open_alerts
FROM next_window_cache ORDER BY satellite_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.NextWindow
	for rows.Next() {
		var nw model.NextWindow
		var typ string
		var nce, nme int64
		var ncs string
		if err := rows.Scan(&nw.SatelliteID, &nce, &ncs, &nme, &typ, &nw.OpenAlerts); err != nil {
			return nil, err
		}
		nw.NextContactEpoch = model.Epoch(nce)
		nw.NextContactStation = ncs
		nw.NextManeuverEpoch = model.Epoch(nme)
		nw.NextManeuverType = model.ManeuverType(typ)
		out = append(out, nw)
	}
	return out, rows.Err()
}

func (q *Queries) ClearNextWindows(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM next_window_cache`)
	return err
}
