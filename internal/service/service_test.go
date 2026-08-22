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

// TestPlanManeuverSameInstantConflict locks the full-flow fix: two maneuvers
// scheduled at the same execution instant on the same satellite must NOT both
// be accepted. Before the unified boundary, PlanManeuver passed from/to=
// execAt+1 to ListActiveManeuvers, a contradiction (executed_at > execAt+1 AND
// executed_at <= execAt+1) that always returned empty, so the conflict check
// saw nothing and the same-instant second maneuver was admitted — producing
// inconsistent scheduling state. With the boundary unified in maneuver.Overlap,
// the second plan must return ErrManeuverConflict.
func TestPlanManeuverSameInstantConflict(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	// Low perigee so perigee_raise is required and always plannable here.
	low, _ := svc.RegisterSatellite(ctx, "LOW", "2", model.Elements{A: 6600, E: 0.12, I: 28.5, Raan: 0, Argp: 0, M: 0, Epoch: 100000})
	const execAt = model.Epoch(100000)
	if _, err := svc.PlanManeuver(ctx, low.ID, model.ManeuverPerigeeRaise, execAt); err != nil {
		t.Fatalf("first plan: %v", err)
	}
	if _, err := svc.PlanManeuver(ctx, low.ID, model.ManeuverPerigeeRaise, execAt); err != model.ErrManeuverConflict {
		t.Fatalf("second plan at same instant: want ErrManeuverConflict, got %v", err)
	}
}

// TestPlanManeuverAdjacentNoConflict locks the companion half of the unified
// boundary: a second maneuver whose execution instant immediately follows the
// first's 1-second window (back-to-back) must be accepted, since half-open
// [Start, End) windows that merely touch do not overlap. The old closed
// interval would have rejected this as a false conflict. PlanPerigeeRaise
// stamps ExecutedAt=now (the execAt arg is ignored), so we advance the fake
// clock by one second between plans to make the windows genuinely adjacent:
// first is [100000,100001), second is [100001,100002).
func TestPlanManeuverAdjacentNoConflict(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "adj_test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	clk := clock.NewFake(100000)
	svc := New(s, clk)
	ctx := context.Background()
	low, _ := svc.RegisterSatellite(ctx, "LOW2", "3", model.Elements{A: 6600, E: 0.12, I: 28.5, Raan: 0, Argp: 0, M: 0, Epoch: 100000})
	if _, err := svc.PlanManeuver(ctx, low.ID, model.ManeuverPerigeeRaise, 100000); err != nil {
		t.Fatalf("first plan: %v", err)
	}
	clk.Advance(1) // next ExecutedAt = 100001 → window [100001,100002), adjacent to the first
	if _, err := svc.PlanManeuver(ctx, low.ID, model.ManeuverPerigeeRaise, 100001); err != nil {
		t.Fatalf("adjacent plan should be accepted: %v", err)
	}
}
