package store

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "orbitops_test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSatelliteCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sat := model.Satellite{
		ID: "sat-1", Name: "TestSat", Catalog: "25544",
		Elements:  model.Elements{A: 7000, E: 0.001, I: 51.6, Raan: 0, Argp: 0, M: 0, Epoch: 1000},
		Status:    model.StatusNominal,
		CreatedAt: 1000, UpdatedAt: 1000,
	}
	if err := s.InTx(ctx, func(q *Queries) error {
		return q.InsertSatellite(ctx, sat)
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.q.GetSatellite(ctx, "sat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Elements.A != 7000 || got.Status != model.StatusNominal {
		t.Fatalf("unexpected: %+v", got)
	}
	lst, err := s.q.ListSatellites(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(lst) != 1 {
		t.Fatalf("list len=%d", len(lst))
	}
}

func TestElementHistoryLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.InTx(ctx, func(q *Queries) error { return q.InsertSatellite(ctx, model.Satellite{ID: "s1", Name: "n", Catalog: "c", Elements: model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}, Status: model.StatusNominal, CreatedAt: 0, UpdatedAt: 0}) })
	for i, ep := range []model.Epoch{1000, 2000, 1500} {
		h := model.ElementHistory{ID: "", SatelliteID: "s1", Elements: model.Elements{A: float64(7000 + i), E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: ep}, Source: model.SourceTLEUpdate, CreatedAt: ep}
		s.InTx(ctx, func(q *Queries) error { return q.InsertElementHistory(ctx, h) })
	}
	latest, err := s.q.LatestElementHistory(ctx, "s1")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Elements.Epoch != 2000 {
		t.Fatalf("latest epoch = %d want 2000", latest.Elements.Epoch)
	}
}

func TestEventOrdering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, ts := range []int64{3000, 1000, 2000} {
		e := model.Event{Type: model.EventTLEUpdate, SatelliteID: "s1", PayloadJSON: "{}", Ts: model.Epoch(ts), CreatedAt: model.Epoch(ts)}
		s.InTx(ctx, func(q *Queries) error { return q.AppendEvent(ctx, e) })
	}
	evs, err := s.q.ListEventsOrdered(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("len=%d", len(evs))
	}
	if evs[0].Ts != 1000 || evs[1].Ts != 2000 || evs[2].Ts != 3000 {
		t.Fatalf("order wrong: %+v", evs)
	}
}

func TestNextWindowUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	nw := model.NextWindow{SatelliteID: "s1", NextContactEpoch: 5000, NextContactStation: "sta", OpenAlerts: 2}
	s.InTx(ctx, func(q *Queries) error { return q.UpsertNextWindow(ctx, nw) })
	nw.OpenAlerts = 3
	s.InTx(ctx, func(q *Queries) error { return q.UpsertNextWindow(ctx, nw) })
	got, err := s.q.GetNextWindow(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OpenAlerts != 3 {
		t.Fatalf("open alerts = %d want 3", got.OpenAlerts)
	}
}

// TestLifecycleStateRoundTrip guards against the regression where contact
// source, maneuver status and alert status were dropped to "" on read, so the
// UI could not tell the real lifecycle stage after a reload.
func TestLifecycleStateRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Contact source survives a read.
	contact := model.Contact{
		ID: "c1", SatelliteID: "s1", StationID: "st1",
		AOS: 1000, Los: 2000, TCA: 1500, MaxElevationDeg: 45,
		AOSAzDeg: 10, LosAzDeg: 350, Sunlit: true,
		Source: model.ContactReplay, ComputedAt: 900,
	}
	if err := s.InTx(ctx, func(q *Queries) error { return q.InsertContact(ctx, contact) }); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	if got, err := s.q.GetContact(ctx, "c1"); err != nil {
		t.Fatalf("get contact: %v", err)
	} else if got.Source != model.ContactReplay {
		t.Fatalf("contact source = %q want %q", got.Source, model.ContactReplay)
	}

	// Maneuver status survives a read.
	maneuver := model.Maneuver{
		ID: "m1", SatelliteID: "s1", Type: model.ManeuverPerigeeRaise,
		PlannedAt: 1000, ExecutedAt: 2000, DeltaVMps: 1.5,
		Target: model.Elements{A: 7000, E: 0.01, I: 28, Raan: 0, Argp: 0, M: 0, Epoch: 1000},
		Status: model.ManeuverExecuting, CreatedAt: 900,
	}
	if err := s.InTx(ctx, func(q *Queries) error { return q.InsertManeuver(ctx, maneuver) }); err != nil {
		t.Fatalf("insert maneuver: %v", err)
	}
	if got, err := s.q.GetManeuver(ctx, "m1"); err != nil {
		t.Fatalf("get maneuver: %v", err)
	} else if got.Status != model.ManeuverExecuting {
		t.Fatalf("maneuver status = %q want %q", got.Status, model.ManeuverExecuting)
	}

	// Alert status survives a read.
	alert := model.CollisionAlert{
		ID: "a1", PrimaryID: "s1", SecondaryID: "s2", TCA: 5000,
		MinDistanceKm: 0.5, CollisionProbability: 0.9,
		Status: model.AlertAvoided, AvoidManeuverID: "m1", CreatedAt: 900,
	}
	if err := s.InTx(ctx, func(q *Queries) error { return q.InsertAlert(ctx, alert) }); err != nil {
		t.Fatalf("insert alert: %v", err)
	}
	if got, err := s.q.GetAlert(ctx, "a1"); err != nil {
		t.Fatalf("get alert: %v", err)
	} else if got.Status != model.AlertAvoided {
		t.Fatalf("alert status = %q want %q", got.Status, model.AlertAvoided)
	}
}
