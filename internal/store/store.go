// Package store is the SQLite persistence layer for orbitops. It owns the
// schema, all CRUD, the event stream, and the ReconcileAll replay support.
//
// To avoid deadlocks under SetMaxOpenConns(1), every read or write that must
// run inside a transaction uses the Tx variant of the query (the *sql.Tx),
// never the global *sql.DB. The DBTX interface lets one Queries type serve
// both connection and transaction. Entity-specific queries live in their own
// files (satellites.go, contacts.go, ...) but all belong to package store.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"orbitops/internal/model"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// DBTX is the common interface over *sql.DB and *sql.Tx. Only the methods the
// Queries use are required.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// Queries runs against a single DBTX (either the pool or a transaction).
type Queries struct {
	db DBTX
}

// Store holds the connection pool and exposes InTx for transactional work.
type Store struct {
	db    *sql.DB
	q     *Queries
	idGen func() string
}

// Open opens (or creates) the SQLite database at path and runs migrations.
// The path is used verbatim (no "file:" prefix), matching modernc.org/sqlite
// expectations for a plain filesystem path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection to avoid concurrent-writer lock contention and the
	// cross-connection deadlock hazard when reads happen inside transactions.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, q: &Queries{db: db}, idGen: defaultIDGen}
	if err := s.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// SetIDGenerator overrides the deterministic ID source (used for replay-stable
// identifiers). Must be set before any insert.
func (s *Store) SetIDGenerator(fn func() string) { s.idGen = fn }

// Close releases the connection pool.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the pool for migrate-only callers; not used by business code.
func (s *Store) DB() *sql.DB { return s.db }

// Q returns the base Queries (pool-backed) for transaction-free reads.
func (s *Store) Q() *Queries { return s.q }

// InTx runs fn inside a database transaction. fn receives a Queries bound to
// the transaction; every read and write inside fn MUST use that tx Queries,
// never the pool, or the single-connection pool deadlocks.
func (s *Store) InTx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	tq := &Queries{db: tx}
	if err := fn(tq); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Migrate creates the schema if absent.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// ID returns a fresh deterministic ID.
func (s *Store) ID() string { return s.idGen() }

// scanner abstracts *sql.Row and *sql.Rows Scan.
type scanner interface {
	Scan(dest ...interface{}) error
}

// Delegated read methods on Store forward to the base Queries. These are safe
// for read-only use outside a transaction; the single-connection pool only
// deadlocks when a transactional read uses the pool instead of the tx.
func (s *Store) GetSatellite(ctx context.Context, id string) (model.Satellite, error) {
	return s.q.GetSatellite(ctx, id)
}
func (s *Store) ListSatellites(ctx context.Context) ([]model.Satellite, error) {
	return s.q.ListSatellites(ctx)
}
func (s *Store) GetStation(ctx context.Context, id string) (model.GroundStation, error) {
	return s.q.GetStation(ctx, id)
}
func (s *Store) ListStations(ctx context.Context) ([]model.GroundStation, error) {
	return s.q.ListStations(ctx)
}
func (s *Store) ListElementHistory(ctx context.Context, satID string) ([]model.ElementHistory, error) {
	return s.q.ListElementHistory(ctx, satID)
}
func (s *Store) LatestElementHistory(ctx context.Context, satID string) (model.ElementHistory, error) {
	return s.q.LatestElementHistory(ctx, satID)
}
func (s *Store) GetContact(ctx context.Context, id string) (model.Contact, error) {
	return s.q.GetContact(ctx, id)
}
func (s *Store) ListContacts(ctx context.Context, satID, stationID string) ([]model.Contact, error) {
	return s.q.ListContacts(ctx, satID, stationID)
}
func (s *Store) GetManeuver(ctx context.Context, id string) (model.Maneuver, error) {
	return s.q.GetManeuver(ctx, id)
}
func (s *Store) ListManeuvers(ctx context.Context, satID string) ([]model.Maneuver, error) {
	return s.q.ListManeuvers(ctx, satID)
}
func (s *Store) GetAlert(ctx context.Context, id string) (model.CollisionAlert, error) {
	return s.q.GetAlert(ctx, id)
}
func (s *Store) ListAlerts(ctx context.Context, status string) ([]model.CollisionAlert, error) {
	return s.q.ListAlerts(ctx, status)
}
func (s *Store) ListEventsOrdered(ctx context.Context) ([]model.Event, error) {
	return s.q.ListEventsOrdered(ctx)
}
func (s *Store) GetNextWindow(ctx context.Context, satID string) (model.NextWindow, error) {
	return s.q.GetNextWindow(ctx, satID)
}
func (s *Store) ListNextWindows(ctx context.Context) ([]model.NextWindow, error) {
	return s.q.ListNextWindows(ctx)
}
func (s *Store) CountOpenAlerts(ctx context.Context) (int, error) {
	return s.q.CountOpenAlerts(ctx)
}

// ResetAll wipes all tables. Used by smoke tests and --migrate-only resets.
func (s *Store) ResetAll(ctx context.Context) error {
	return s.InTx(ctx, func(q *Queries) error {
		for _, t := range []string{"next_window_cache", "events", "collision_alerts", "maneuvers", "contacts", "element_history", "ground_stations", "satellites"} {
			if _, err := q.db.ExecContext(ctx, `DELETE FROM `+t); err != nil {
				return err
			}
		}
		return nil
	})
}

// --------------------------------------------------------------------------- //
// helpers
// --------------------------------------------------------------------------- //

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nextID is a package-level deterministic fallback used when an ID field is
// empty at insert time.
var nextID = defaultIDGen

// defaultIDGen is a monotonic counter; replaced by idlib in production wiring.
var (
	defaultIDGen = func() string {
		idMu.Lock()
		defer idMu.Unlock()
		globalSeq++
		return formatGlobalID(globalSeq)
	}
	idMu      sync.Mutex
	globalSeq int64
)

func formatGlobalID(n int64) string {
	const w = 16
	var b [w]byte
	for i := w - 1; i >= 0; i-- {
		b[i] = byte('0' + int(n%10))
		n /= 10
	}
	return string(b[:])
}

// EncodePayload marshals v to JSON for event storage.
func EncodePayload(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// errNoRows reports whether err is sql.ErrNoRows, used to map to domain
// not-found errors in the scan helpers.
func errNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
