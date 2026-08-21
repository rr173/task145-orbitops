package store

import (
	"context"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Satellites
// --------------------------------------------------------------------------- //

func (q *Queries) InsertSatellite(ctx context.Context, sat model.Satellite) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO satellites (id,name,catalog,a,e,i,raan,argp,m,epoch_sec,status,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sat.ID, sat.Name, sat.Catalog, sat.Elements.A, sat.Elements.E, sat.Elements.I,
		sat.Elements.Raan, sat.Elements.Argp, sat.Elements.M, int64(sat.Elements.Epoch),
		string(sat.Status), int64(sat.CreatedAt), int64(sat.UpdatedAt))
	return err
}

func (q *Queries) UpdateSatelliteElements(ctx context.Context, id string, el model.Elements, status model.SatelliteStatus, now model.Epoch) error {
	_, err := q.db.ExecContext(ctx, `
UPDATE satellites SET a=?,e=?,i=?,raan=?,argp=?,m=?,epoch_sec=?,status=?,updated_at=? WHERE id=?`,
		el.A, el.E, el.I, el.Raan, el.Argp, el.M, int64(el.Epoch), string(status), int64(now), id)
	return err
}

func (q *Queries) UpdateSatelliteStatus(ctx context.Context, id string, status model.SatelliteStatus, now model.Epoch) error {
	_, err := q.db.ExecContext(ctx, `UPDATE satellites SET status=?,updated_at=? WHERE id=?`, string(status), int64(now), id)
	return err
}

func (q *Queries) GetSatellite(ctx context.Context, id string) (model.Satellite, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,name,catalog,a,e,i,raan,argp,m,epoch_sec,status,created_at,updated_at FROM satellites WHERE id=?`, id)
	return scanSatellite(row)
}

func (q *Queries) ListSatellites(ctx context.Context) ([]model.Satellite, error) {
	rows, err := q.db.QueryContext(ctx, `
SELECT id,name,catalog,a,e,i,raan,argp,m,epoch_sec,status,created_at,updated_at FROM satellites ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Satellite
	for rows.Next() {
		s, err := scanSatellite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (q *Queries) DeleteSatellite(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM satellites WHERE id=?`, id)
	return err
}

func scanSatellite(sc scanner) (model.Satellite, error) {
	var s model.Satellite
	var status string
	var ep, ca, ua int64
	err := sc.Scan(&s.ID, &s.Name, &s.Catalog, &s.Elements.A, &s.Elements.E, &s.Elements.I,
		&s.Elements.Raan, &s.Elements.Argp, &s.Elements.M, &ep, &status, &ca, &ua)
	if err != nil {
		if errNoRows(err) {
			return model.Satellite{}, model.ErrSatelliteNotFound
		}
		return model.Satellite{}, err
	}
	s.Elements.Epoch = model.Epoch(ep)
	s.Status = model.SatelliteStatus(status)
	s.CreatedAt = model.Epoch(ca)
	s.UpdatedAt = model.Epoch(ua)
	return s, nil
}
