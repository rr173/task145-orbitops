package store

import (
	"context"
	"database/sql"
	"errors"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Maneuvers
// --------------------------------------------------------------------------- //

func (q *Queries) InsertManeuver(ctx context.Context, m model.Maneuver) error {
	id := m.ID
	if id == "" {
		id = nextID()
	}
	_, err := q.db.ExecContext(ctx, `
INSERT INTO maneuvers (id,satellite_id,type,planned_at,executed_at,delta_v_mps,target_a,target_e,target_i,target_raan,target_argp,target_m,status,reason_alert_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, m.SatelliteID, string(m.Type), int64(m.PlannedAt), int64(m.ExecutedAt),
		m.DeltaVMps, m.Target.A, m.Target.E, m.Target.I, m.Target.Raan, m.Target.Argp, m.Target.M,
		string(m.Status), nullableString(m.ReasonAlertID), int64(m.CreatedAt))
	return err
}

func (q *Queries) UpdateManeuverStatus(ctx context.Context, id string, status model.ManeuverStatus, now model.Epoch) error {
	_, err := q.db.ExecContext(ctx, `UPDATE maneuvers SET status=? WHERE id=?`, string(status), id)
	return err
}

func (q *Queries) GetManeuver(ctx context.Context, id string) (model.Maneuver, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,satellite_id,type,planned_at,executed_at,delta_v_mps,target_a,target_e,target_i,target_raan,target_argp,target_m,status,reason_alert_id,created_at
FROM maneuvers WHERE id=?`, id)
	return scanManeuver(row)
}

func (q *Queries) ListManeuvers(ctx context.Context, satID string) ([]model.Maneuver, error) {
	qry := `SELECT id,satellite_id,type,planned_at,executed_at,delta_v_mps,target_a,target_e,target_i,target_raan,target_argp,target_m,status,reason_alert_id,created_at FROM maneuvers`
	args := []interface{}{}
	if satID != "" {
		qry += ` WHERE satellite_id=?`
		args = append(args, satID)
	}
	qry += ` ORDER BY executed_at ASC`
	rows, err := q.db.QueryContext(ctx, qry, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Maneuver
	for rows.Next() {
		m, err := scanManeuver(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListActiveManeuvers returns all maneuvers for a satellite with status
// planned or executing and executed_at within [from,to]. Used for the
// maneuver conflict check.
func (q *Queries) ListActiveManeuvers(ctx context.Context, satID string, from, to model.Epoch) ([]model.Maneuver, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT id,satellite_id,type,planned_at,executed_at,delta_v_mps,target_a,target_e,target_i,target_raan,target_argp,target_m,status,reason_alert_id,created_at
FROM maneuvers WHERE satellite_id=? AND status IN ('planned','executing') AND executed_at>=? AND executed_at<=?
ORDER BY executed_at ASC`, satID, int64(from), int64(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Maneuver
	for rows.Next() {
		m, err := scanManeuver(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanManeuver(sc scanner) (model.Maneuver, error) {
	var m model.Maneuver
	var typ, status string
	var reason sql.NullString
	var pa, ea, ca int64
	err := sc.Scan(&m.ID, &m.SatelliteID, &typ, &pa, &ea, &m.DeltaVMps,
		&m.Target.A, &m.Target.E, &m.Target.I, &m.Target.Raan, &m.Target.Argp, &m.Target.M,
		&status, &reason, &ca)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Maneuver{}, model.ErrManeuverNotFound
		}
		return model.Maneuver{}, err
	}
	m.Type = model.ManeuverType(typ)
	m.Status = model.ManeuverStatus(status)
	m.PlannedAt = model.Epoch(pa)
	m.ExecutedAt = model.Epoch(ea)
	m.CreatedAt = model.Epoch(ca)
	if reason.Valid {
		m.ReasonAlertID = reason.String
	}
	return m, nil
}
