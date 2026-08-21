package store

import (
	"context"
	"errors"
	"database/sql"

	"orbitops/internal/model"
)

// --------------------------------------------------------------------------- //
// Contacts
// --------------------------------------------------------------------------- //

func (q *Queries) InsertContact(ctx context.Context, c model.Contact) error {
	var id string
	if c.ID != "" {
		id = c.ID
	} else {
		id = nextID()
	}
	_, err := q.db.ExecContext(ctx, `
INSERT INTO contacts (id,satellite_id,station_id,aos,los,tca,max_elevation_deg,aos_az_deg,los_az_deg,sunlit,source,computed_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, id, c.SatelliteID, c.StationID, int64(c.AOS), int64(c.Los), int64(c.TCA),
		c.MaxElevationDeg, c.AOSAzDeg, c.LosAzDeg, boolToInt(c.Sunlit), string(c.Source), int64(c.ComputedAt))
	return err
}

// ReplaceContactsForWindow deletes all contacts for a satellite/station pair
// whose AOS falls within [start,end] and is used during replay to overwrite
// recomputed windows with the new baseline.
func (q *Queries) ReplaceContactsForWindow(ctx context.Context, satID, stationID string, start, end model.Epoch) error {
	_, err := q.db.ExecContext(ctx, `
DELETE FROM contacts WHERE satellite_id=? AND station_id=? AND aos>=? AND aos<=?`,
		satID, stationID, int64(start), int64(end))
	return err
}

func (q *Queries) GetContact(ctx context.Context, id string) (model.Contact, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,satellite_id,station_id,aos,los,tca,max_elevation_deg,aos_az_deg,los_az_deg,sunlit,source,computed_at
FROM contacts WHERE id=?`, id)
	return scanContact(row)
}

func (q *Queries) ListContacts(ctx context.Context, satID, stationID string) ([]model.Contact, error) {
	qry := `SELECT id,satellite_id,station_id,aos,los,tca,max_elevation_deg,aos_az_deg,los_az_deg,sunlit,source,computed_at FROM contacts`
	args := []interface{}{}
	if satID != "" && stationID != "" {
		qry += ` WHERE satellite_id=? AND station_id=?`
		args = append(args, satID, stationID)
	} else if satID != "" {
		qry += ` WHERE satellite_id=?`
		args = append(args, satID)
	} else if stationID != "" {
		qry += ` WHERE station_id=?`
		args = append(args, stationID)
	}
	qry += ` ORDER BY aos ASC`
	rows, err := q.db.QueryContext(ctx, qry, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// NextContactAfter returns the earliest contact for a satellite with AOS >=
// since. Used to compute the "next contact" cache entry.
func (q *Queries) NextContactAfter(ctx context.Context, satID string, since model.Epoch) (model.Contact, error) {
	row := q.db.QueryRowContext(ctx, `
SELECT id,satellite_id,station_id,aos,los,tca,max_elevation_deg,aos_az_deg,los_az_deg,sunlit,source,computed_at
FROM contacts WHERE satellite_id=? AND aos>=? ORDER BY aos ASC LIMIT 1`, satID, int64(since))
	return scanContact(row)
}

func scanContact(sc scanner) (model.Contact, error) {
	var c model.Contact
	var src string
	var sun int
	var aos, los, tca, ca int64
	err := sc.Scan(&c.ID, &c.SatelliteID, &c.StationID, &aos, &los, &tca, &c.MaxElevationDeg,
		&c.AOSAzDeg, &c.LosAzDeg, &sun, &src, &ca)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Contact{}, model.ErrContactNotFound
		}
		return model.Contact{}, err
	}
	c.AOS = model.Epoch(aos)
	c.Los = model.Epoch(los)
	c.TCA = model.Epoch(tca)
	c.ComputedAt = model.Epoch(ca)
	c.Sunlit = sun != 0
	c.Source = ""
	return c, nil
}
