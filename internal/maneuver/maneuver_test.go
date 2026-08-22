package maneuver

import (
	"testing"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

func TestEWNotRequiredForLEO(t *testing.T) {
	pl := New(propagator.New())
	el := model.Elements{A: 7000, E: 0.001, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	dec, err := pl.EvaluateEW(el, 0)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if dec.Required {
		t.Fatal("LEO should not trigger EW")
	}
}

func TestEWLeadTime(t *testing.T) {
	pl := New(propagator.New())
	// Construct a GEO element set whose drift exceeds the deadband. We force a
	// drift by setting the semi-major axis off-nominal: a too large by 20 km
	// drifts eastward ~ a few deg/day well beyond 0.05.
	el := model.Elements{A: orbmath.AGeo + 20, E: 0.001, I: 0.1, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	dec, err := pl.EvaluateEW(el, 0)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if !dec.Required {
		t.Fatalf("expected EW required for off-nominal GEO")
	}
	// crossing at 0, latest = 0 + 0.5d
	if dec.LatestExecAt != model.Epoch(orbmath.EWLeadTimeDays*86400) {
		t.Fatalf("latest exec = %v want %v", dec.LatestExecAt, orbmath.EWLeadTimeDays*86400)
	}
	// executing 0.6d later must fail
	if err := ValidateExecTime(dec, dec.LatestExecAt+model.Epoch(0.1*86400)); err == nil {
		t.Fatal("expected ErrManeuverTooLate")
	}
	// executing 0.4d later must pass
	if err := ValidateExecTime(dec, dec.LatestExecAt-model.Epoch(0.1*86400)); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestOverlap(t *testing.T) {
	a := Window{Start: 10, End: 20}
	b := Window{Start: 15, End: 25}
	if !Overlap(a, b) {
		t.Fatal("overlapping windows reported disjoint")
	}
	c := Window{Start: 20, End: 30}
	if Overlap(a, c) {
		t.Fatal("touching-at-end windows should not overlap")
	}
}

// TestOverlapSameInstant locks the first symptom of the unified-boundary fix:
// two maneuvers sharing an execution instant MUST conflict. With the old
// closed-interval boundary in Overlap paired with the SQL >…<= pre-filter,
// same-instant maneuvers could slip past the conflict check (the historical
// PlanManeuver query passed from=execAt+1,to=execAt+1, a contradiction that
// always returned empty). Half-open [Start, End) makes same-start windows
// overlap by construction.
func TestOverlapSameInstant(t *testing.T) {
	w := Window{Start: 100, End: 101}
	if !Overlap(w, w) {
		t.Fatal("identical windows must overlap")
	}
	// two maneuvers scheduled at the same execution epoch
	m1 := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 100, Status: model.ManeuverPlanned}
	m2 := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 100, Status: model.ManeuverPlanned}
	if !Overlap(ManeuverWindow(m1), ManeuverWindow(m2)) {
		t.Fatal("same-instant maneuvers must be flagged as conflicting")
	}
}

// TestOverlapAdjacentNotConflict locks the second symptom: adjacent windows
// (one's End equals the other's Start) must NOT conflict — they represent
// back-to-back maneuvers. The old closed boundary reported these as overlapping.
func TestOverlapAdjacentNotConflict(t *testing.T) {
	m1 := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 100, Status: model.ManeuverPlanned}
	m2 := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 101, Status: model.ManeuverPlanned} // starts exactly when m1 ends
	if Overlap(ManeuverWindow(m1), ManeuverWindow(m2)) {
		t.Fatal("adjacent (back-to-back) maneuvers must not conflict")
	}
	// reverse order should also be conflict-free
	if Overlap(ManeuverWindow(m2), ManeuverWindow(m1)) {
		t.Fatal("adjacent (back-to-back) maneuvers must not conflict (reversed)")
	}
}

func TestCheckConflict(t *testing.T) {
	planned := model.Maneuver{SatelliteID: "s1", PlannedAt: 100, ExecutedAt: 100, Status: model.ManeuverPlanned}
	existing := []model.Maneuver{
		{SatelliteID: "s1", PlannedAt: 100, ExecutedAt: 100, Status: model.ManeuverPlanned},
	}
	if err := CheckConflictAgainstExisting(planned, existing); err == nil {
		t.Fatal("expected conflict")
	}
	existing2 := []model.Maneuver{
		{SatelliteID: "s2", PlannedAt: 100, Status: model.ManeuverPlanned},
		{SatelliteID: "s1", PlannedAt: 1000, Status: model.ManeuverPlanned},
		{SatelliteID: "s1", PlannedAt: 100, Status: model.ManeuverDone},
	}
	if err := CheckConflictAgainstExisting(planned, existing2); err != nil {
		t.Fatalf("expected no conflict, got %v", err)
	}
}

// TestCheckConflictSameInstantPlanned exercises the full conflict path
// (ManeuverWindow + Overlap) for the "two same-instant planned maneuvers"
// case the unified boundary is meant to catch.
func TestCheckConflictSameInstantPlanned(t *testing.T) {
	planned := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 500, Status: model.ManeuverPlanned}
	existing := []model.Maneuver{
		{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 500, Status: model.ManeuverPlanned},
	}
	if err := CheckConflictAgainstExisting(planned, existing); err != model.ErrManeuverConflict {
		t.Fatalf("expected ErrManeuverConflict for same-instant maneuvers, got %v", err)
	}
}

// TestCheckConflictAdjacentNotConflict exercises the full conflict path for
// back-to-back maneuvers: the second starts exactly when the first's window
// ends, so there must be no conflict.
func TestCheckConflictAdjacentNotConflict(t *testing.T) {
	planned := model.Maneuver{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 500, Status: model.ManeuverPlanned} // window [500,501)
	existing := []model.Maneuver{
		{SatelliteID: "s1", PlannedAt: 0, ExecutedAt: 499, Status: model.ManeuverPlanned}, // window [499,500) — touches, no overlap
	}
	if err := CheckConflictAgainstExisting(planned, existing); err != nil {
		t.Fatalf("expected no conflict for adjacent maneuvers, got %v", err)
	}
}

func TestPlanCollisionAvoidanceInsufficient(t *testing.T) {
	_, err := PlanCollisionAvoidance("s1", "a1", 0.1, 5.0, 100, model.Elements{}, 0)
	if err != model.ErrAvoidanceInsufficient {
		t.Fatalf("expected ErrAvoidanceInsufficient got %v", err)
	}
	m, err := PlanCollisionAvoidance("s1", "a1", 0.5, 25.0, 100, model.Elements{A: 7000}, 0)
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if m.ReasonAlertID != "a1" {
		t.Fatalf("reason alert id not set")
	}
}
