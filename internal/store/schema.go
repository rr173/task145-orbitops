package store

// schema is the full SQLite DDL for orbitops. It is idempotent (IF NOT
// EXISTS) so ReconcileAll and re-open are safe.
const schema = `
CREATE TABLE IF NOT EXISTS satellites (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    catalog TEXT NOT NULL,
    a REAL NOT NULL,
    e REAL NOT NULL,
    i REAL NOT NULL,
    raan REAL NOT NULL,
    argp REAL NOT NULL,
    m REAL NOT NULL,
    epoch_sec INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS ground_stations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    lat_deg REAL NOT NULL,
    lon_deg REAL NOT NULL,
    alt_m REAL NOT NULL,
    min_elevation_deg REAL NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS element_history (
    id TEXT PRIMARY KEY,
    satellite_id TEXT NOT NULL,
    a REAL NOT NULL,
    e REAL NOT NULL,
    i REAL NOT NULL,
    raan REAL NOT NULL,
    argp REAL NOT NULL,
    m REAL NOT NULL,
    epoch_sec INTEGER NOT NULL,
    source TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_elem_sat ON element_history(satellite_id, epoch_sec);
CREATE TABLE IF NOT EXISTS contacts (
    id TEXT PRIMARY KEY,
    satellite_id TEXT NOT NULL,
    station_id TEXT NOT NULL,
    aos INTEGER NOT NULL,
    los INTEGER NOT NULL,
    tca INTEGER NOT NULL,
    max_elevation_deg REAL NOT NULL,
    aos_az_deg REAL NOT NULL,
    los_az_deg REAL NOT NULL,
    sunlit INTEGER NOT NULL,
    source TEXT NOT NULL,
    computed_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contact_sat ON contacts(satellite_id, aos);
CREATE INDEX IF NOT EXISTS idx_contact_sta ON contacts(station_id, aos);
CREATE TABLE IF NOT EXISTS maneuvers (
    id TEXT PRIMARY KEY,
    satellite_id TEXT NOT NULL,
    type TEXT NOT NULL,
    planned_at INTEGER NOT NULL,
    executed_at INTEGER NOT NULL,
    delta_v_mps REAL NOT NULL,
    target_a REAL NOT NULL,
    target_e REAL NOT NULL,
    target_i REAL NOT NULL,
    target_raan REAL NOT NULL,
    target_argp REAL NOT NULL,
    target_m REAL NOT NULL,
    status TEXT NOT NULL,
    reason_alert_id TEXT,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_man_sat ON maneuvers(satellite_id, executed_at);
CREATE TABLE IF NOT EXISTS collision_alerts (
    id TEXT PRIMARY KEY,
    primary_id TEXT NOT NULL,
    secondary_id TEXT NOT NULL,
    tca INTEGER NOT NULL,
    min_distance_km REAL NOT NULL,
    collision_probability REAL NOT NULL,
    status TEXT NOT NULL,
    avoid_maneuver_id TEXT,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_alert_status ON collision_alerts(status);
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    satellite_id TEXT,
    payload_json TEXT NOT NULL,
    ts_sec INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts_sec);
CREATE TABLE IF NOT EXISTS next_window_cache (
    satellite_id TEXT PRIMARY KEY,
    next_contact_epoch INTEGER NOT NULL,
    next_contact_station TEXT NOT NULL,
    next_maneuver_epoch INTEGER NOT NULL,
    next_maneuver_type TEXT NOT NULL,
    open_alerts INTEGER NOT NULL
);
`
