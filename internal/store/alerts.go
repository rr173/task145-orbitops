package store

import (
	"context"
	"database/sql"
	"errors"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Collision alerts
// --------------------------------------------------------------------------- //

func (q *Queries) InsertAlert(ctx context.Context, a model.CollisionAlert) error {
	id := a.ID
	if id == "" {
		id = nextID()
	}
	_, err := q.db.ExecContext(ctx, `
INSERT INTO collision_alerts (id,primary_id,secondary_id,tca,min_distance_km,collision_probability,status,avoid_maneuver_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?)`, id, a.PrimaryID, a.SecondaryID, int64(a.TCA), a.MinDistanceKm,
		a.CollisionProbability, string(a.Status), nullableString(a.AvoidManeuverID), int64(a.CreatedAt))
	return err
}

func (q *Queries) SetAlertStatus(ctx context.Context, id string, status model.AlertStatus, avoidManeuverID string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE collision_alerts SET status=?, avoid_maneuver_id=? WHERE id=?`,
		string(status), nullableString(avoidManeuverID), id)
	return err
}

func (q *Queries) GetAlert(ctx context.Context, id string) (model.CollisionAlert, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,primary_id,secondary_id,tca,min_distance_km,collision_probability,status,avoid_maneuver_id,created_at
FROM collision_alerts WHERE id=?`, id)
	return scanAlert(row)
}

func (q *Queries) ListAlerts(ctx context.Context, status string) ([]model.CollisionAlert, error) {
	qry := `SELECT id,primary_id,secondary_id,tca,min_distance_km,collision_probability,status,avoid_maneuver_id,created_at FROM collision_alerts`
	args := []interface{}{}
	if status != "" {
		qry += ` WHERE status=?`
		args = append(args, status)
	}
	qry += ` ORDER BY tca ASC`
	rows, err := q.db.QueryContext(ctx, qry, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CollisionAlert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanAlert(sc scanner) (model.CollisionAlert, error) {
	var a model.CollisionAlert
	var status string
	var avoid sql.NullString
	var tca, ca int64
	err := sc.Scan(&a.ID, &a.PrimaryID, &a.SecondaryID, &tca, &a.MinDistanceKm, &a.CollisionProbability, &status, &avoid, &ca)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.CollisionAlert{}, model.ErrAlertNotFound
		}
		return model.CollisionAlert{}, err
	}
	a.TCA = model.Epoch(tca)
	a.CreatedAt = model.Epoch(ca)
	a.Status = model.AlertStatus(status)
	if avoid.Valid {
		a.AvoidManeuverID = avoid.String
	}
	return a, nil
}

func (q *Queries) CountOpenAlerts(ctx context.Context) (int, error) {
	row := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM collision_alerts WHERE status='open'`)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
