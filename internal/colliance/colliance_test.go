package colliance

import (
	"testing"

	"orbitops/internal/model"
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
