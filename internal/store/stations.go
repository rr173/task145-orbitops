package store

import (
	"context"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Ground stations
// --------------------------------------------------------------------------- //

func (q *Queries) InsertStation(ctx context.Context, st model.GroundStation) error {
	_, err := q.db.ExecContext(ctx, `
INSERT INTO ground_stations (id,name,lat_deg,lon_deg,alt_m,min_elevation_deg,created_at)
VALUES (?,?,?,?,?,?,?)`, st.ID, st.Name, st.LatDeg, st.LonDeg, st.AltM, st.MinElevationDeg, int64(st.CreatedAt))
	return err
}

func (q *Queries) GetStation(ctx context.Context, id string) (model.GroundStation, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,name,lat_deg,lon_deg,alt_m,min_elevation_deg,created_at FROM ground_stations WHERE id=?`, id)
	var st model.GroundStation
	var ca int64
	err := row.Scan(&st.ID, &st.Name, &st.LatDeg, &st.LonDeg, &st.AltM, &st.MinElevationDeg, &ca)
	if err != nil {
		if errNoRows(err) {
			return model.GroundStation{}, model.ErrStationNotFound
		}
		return model.GroundStation{}, err
	}
	st.CreatedAt = model.Epoch(ca)
	return st, nil
}

func (q *Queries) ListStations(ctx context.Context) ([]model.GroundStation, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT id,name,lat_deg,lon_deg,alt_m,min_elevation_deg,created_at FROM ground_stations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.GroundStation
	for rows.Next() {
		var st model.GroundStation
		var ca int64
		if err := rows.Scan(&st.ID, &st.Name, &st.LatDeg, &st.LonDeg, &st.AltM, &st.MinElevationDeg, &ca); err != nil {
			return nil, err
		}
		st.CreatedAt = model.Epoch(ca)
		out = append(out, st)
	}
	return out, rows.Err()
}
