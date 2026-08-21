package orbmath_test

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/clock"
	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
	"orbitops/internal/service"
	"orbitops/internal/store"
)

func TestBug01_AntimeridianNormalizationStaysCanonical(t *testing.T) {
	if got := orbmath.NormalizeDeg(180); got != -180 {
		t.Fatalf("canonical 180-degree angle = %v, want -180", got)
	}
	advanced, err := propagator.New().AdvancedElements(model.Elements{A: 7000, E: 0.01, I: 30, Raan: 180, Epoch: 500}, 500)
	if err != nil {
		t.Fatalf("advance elements: %v", err)
	}
	if advanced.Raan != -180 {
		t.Fatalf("propagated RAAN = %v, want canonical -180", advanced.Raan)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "orbitops.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	svc := service.New(s, clock.NewFake(500))
	station, err := svc.RegisterStation(context.Background(), "date-line", 0, 180, 0, 5)
	if err != nil {
		t.Fatalf("register station: %v", err)
	}
	if station.LonDeg != -180 {
		t.Fatalf("stored station longitude = %v, want canonical -180", station.LonDeg)
	}
}
