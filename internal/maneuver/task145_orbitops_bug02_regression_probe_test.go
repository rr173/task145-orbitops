package maneuver_test

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/clock"
	"orbitops/internal/colliance"
	"orbitops/internal/maneuver"
	"orbitops/internal/model"
	"orbitops/internal/propagator"
	"orbitops/internal/service"
	"orbitops/internal/store"
)

func TestBug02_ClosedOperationalLimitsRemainValid(t *testing.T) {
	decision := &maneuver.EWDecision{Required: true, LatestExecAt: 1000}
	if err := maneuver.ValidateExecTime(decision, 1000); err != nil {
		t.Fatalf("maneuver at the published deadline was rejected: %v", err)
	}
	p := colliance.New(propagator.New())
	_, post, err := p.PlanAvoidance(model.Elements{}, model.Elements{}, &colliance.EvalResult{TCA: 100, MinDistanceKm: 10}, 0)
	if err != nil || post != 20 {
		t.Fatalf("avoidance that reaches the safe boundary was rejected: post=%v err=%v", post, err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "orbitops.db"))
	if err != nil { t.Fatalf("open store: %v", err) }
	t.Cleanup(func() { _ = s.Close() })
	station, err := service.New(s, clock.NewFake(1)).RegisterStation(context.Background(), "polar", 90, 0, 0, 5)
	if err != nil || station.LatDeg != 90 {
		t.Fatalf("station at the supported pole was rejected: station=%+v err=%v", station, err)
	}
}
