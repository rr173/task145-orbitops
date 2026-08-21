package service

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/clock"
	"orbitops/internal/model"
	"orbitops/internal/store"
)

func newSvc(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "svc_test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	clk := clock.NewFake(100000) // start 100000s after J2000
	svc := New(s, clk)
	return svc, s
}

func TestRegisterAndForecast(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	sat, err := svc.RegisterSatellite(ctx, "LEO1", "25544", model.Elements{
		A: 6800, E: 0.001, I: 51.6, Raan: 0, Argp: 0, M: 0, Epoch: 100000,
	})
	if err != nil {
		t.Fatalf("register sat: %v", err)
	}
	st, err := svc.RegisterStation(ctx, "Equator", 0, 0, 0, 5)
	if err != nil {
		t.Fatalf("register station: %v", err)
	}
	contacts, err := svc.ForecastContacts(ctx, sat.ID, st.ID, 100000, 6*3600, 60)
	if err != nil {
		t.Fatalf("forecast: %v", err)
	}
	if len(contacts) == 0 {
		t.Fatalf("expected at least one contact")
	}
}

func TestReconcileIdempotent(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	sat, _ := svc.RegisterSatellite(ctx, "LEO1", "25544", model.Elements{
		A: 6800, E: 0.001, I: 51.6, Raan: 0, Argp: 0, M: 0, Epoch: 100000,
	})
	st, _ := svc.RegisterStation(ctx, "Equator", 0, 0, 0, 5)
	_, err := svc.ForecastContacts(ctx, sat.ID, st.ID, 100000, 6*3600, 60)
	if err != nil {
		t.Fatalf("forecast: %v", err)
	}
	if err := svc.ReconcileAll(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	snap1, _ := svc.SnapshotNextWindows(ctx)
	if err := svc.ReconcileAll(ctx); err != nil {
		t.Fatalf("reconcile2: %v", err)
	}
	snap2, _ := svc.SnapshotNextWindows(ctx)
	if len(snap1) != len(snap2) {
		t.Fatalf("idempotency broken: sizes %d vs %d", len(snap1), len(snap2))
	}
	for k, v := range snap1 {
		v2, ok := snap2[k]
		if !ok {
			t.Fatalf("key %s missing after second reconcile", k)
		}
		if v != v2 {
			t.Fatalf("idempotency broken for %s: %+v vs %+v", k, v, v2)
		}
	}
}

func TestStatusTransitions(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	sat, _ := svc.RegisterSatellite(ctx, "S", "1", model.Elements{A: 6800, E: 0.001, I: 51.6, Epoch: 100000})
	if _, err := svc.SetStatus(ctx, sat.ID, model.StatusRetired); err != nil {
		t.Fatalf("nominal->retired should be allowed: %v", err)
	}
	if _, err := svc.SetStatus(ctx, sat.ID, model.StatusActive); err == nil {
		t.Fatal("retired->active must be rejected")
	}
}

func TestCollisionEvaluateNoAlertWhenFar(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	a, _ := svc.RegisterSatellite(ctx, "A", "1", model.Elements{A: 6800, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 100000})
	b, _ := svc.RegisterSatellite(ctx, "B", "2", model.Elements{A: 6800, E: 0, I: 0, Raan: 0, Argp: 0, M: 180, Epoch: 100000})
	res, alert, err := svc.EvaluateCollision(ctx, a.ID, b.ID, 100000, 6000, 60)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if res.MinDistanceKm < 13000 {
		t.Fatalf("expected far, got %v", res.MinDistanceKm)
	}
	if alert != nil {
		t.Fatalf("no alert expected, got %+v", alert)
	}
}

// TestStationLongitudeCanonicalAntimeridian locks the date-line fix at the
// service boundary: a ground station entered at +180 (east-of-antimeridian
// convention) and one entered at -180 (west-of-antimeridian convention) are the
// same physical station and must be persisted with one canonical longitude,
// matching the longitude representation produced by propagation.
func TestStationLongitudeCanonicalAntimeridian(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	stA, err := svc.RegisterStation(ctx, "Antimeridian-E", 0, 180, 0, 5)
	if err != nil {
		t.Fatalf("register +180: %v", err)
	}
	stB, err := svc.RegisterStation(ctx, "Antimeridian-W", 0, -180, 0, 5)
	if err != nil {
		t.Fatalf("register -180: %v", err)
	}
	if stA.LonDeg != stB.LonDeg {
		t.Fatalf("antimeridian station longitudes differ: +%v vs %v (same place must canonicalize)", stA.LonDeg, stB.LonDeg)
	}
	if stA.LonDeg != -180 {
		t.Fatalf("canonical antimeridian longitude = %v, want -180", stA.LonDeg)
	}
	// persisted value round-trips from the store unchanged.
	got, err := svc.store.GetStation(ctx, stA.ID)
	if err != nil {
		t.Fatalf("get station: %v", err)
	}
	if got.LonDeg != -180 {
		t.Fatalf("persisted longitude = %v, want -180", got.LonDeg)
	}
}

