package colliance

import (
	"math"
	"testing"

	"orbitops/internal/maneuver"
	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

func TestEvaluateSameObject(t *testing.T) {
	pr := New(propagator.New())
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	res, err := pr.Evaluate(el, el, 0, 6000, 60)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if res.MinDistanceKm > 1e-6 {
		t.Fatalf("same object distance should be ~0 got %v", res.MinDistanceKm)
	}
}

func TestEvaluateDistantObjects(t *testing.T) {
	pr := New(propagator.New())
	// Two satellites in the same plane but opposite phase -> minimum distance ~2a
	a := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	b := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 180, Epoch: 0}
	res, err := pr.Evaluate(a, b, 0, 6000, 60)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if res.MinDistanceKm < 13000 {
		t.Fatalf("expected >13000km got %v", res.MinDistanceKm)
	}
}

func TestPlanAvoidanceAlreadySafe(t *testing.T) {
	pr := New(propagator.New())
	a := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	b := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 180, Epoch: 0}
	res, _ := pr.Evaluate(a, b, 0, 6000, 60)
	dv, post, err := pr.PlanAvoidance(a, b, res, 100)
	if err != nil {
		t.Fatalf("already-safe should not error, got %v", err)
	}
	if dv != 0 {
		t.Fatalf("dv should be 0 for already-safe, got %v", dv)
	}
	_ = post
}

func TestPlanAvoidanceInsufficientAtTCA(t *testing.T) {
	pr := New(propagator.New())
	a := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	b := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	res, _ := pr.Evaluate(a, b, 0, 6000, 60)
	// burn after TCA (execAt > TCA) -> insufficient
	_, _, err := pr.PlanAvoidance(a, b, res, res.TCA+10)
	if err == nil {
		t.Fatal("expected error for burn at/after TCA")
	}
}

// TestPlanAvoidanceJustReachesSafeThreshold verifies the published-safe
// boundary: when the planned burn raises the post-burn miss distance to
// exactly CollisionSafeKm, the plan must be accepted (not rejected) and the
// reported margin must be exactly zero — never a shaved, inconsistent
// remainder. This guards the cross-layer consistency of the safe threshold
// between colliance.PlanAvoidance and maneuver.PlanCollisionAvoidance.
func TestPlanAvoidanceJustReachesSafeThreshold(t *testing.T) {
	pr := New(propagator.New())
	execAt := model.Epoch(100)
	res := &EvalResult{
		TCA:            execAt + 1000,
		MinDistanceKm:  orbmath.CollisionSafeKm - 5.0, // 5 km short of the safe threshold
	}

	dv, post, err := pr.PlanAvoidance(model.Elements{A: 7000}, model.Elements{A: 7000}, res, execAt)
	if err != nil {
		t.Fatalf("just-safe avoidance must be accepted, got err %v", err)
	}
	if dv <= 0 {
		t.Fatalf("expected non-zero delta-v for a deficient conjunction, got %v", dv)
	}
	// post must be exactly the safe threshold: no shaved margin, no overshoot.
	if math.Abs(post-orbmath.CollisionSafeKm) > 1e-9 {
		t.Fatalf("post-burn distance must equal CollisionSafeKm, got %v (want %v)",
			post, orbmath.CollisionSafeKm)
	}

	// The maneuver layer must agree: an exactly-safe post distance is accepted.
	m, err := maneuver.PlanCollisionAvoidance("s1", "a1", dv, post, execAt, model.Elements{A: 7000}, 0)
	if err != nil {
		t.Fatalf("maneuver.PlanCollisionAvoidance must accept exactly-safe distance, got %v", err)
	}
	if m.ReasonAlertID != "a1" {
		t.Fatalf("maneuver not linked to alert: %+v", m)
	}
}
