package propagator_test

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

func TestBug05_ParabolicElementsAreRejectedBeforePropagation(t *testing.T) {
	el := model.Elements{A: 7000, E: 1}
	if err := propagator.Validate(el); err != model.ErrInvalidElements { t.Fatalf("validate e=1: %v", err) }
	if _, err := orbmath.SolveKepler(0.1, 1); err != model.ErrInvalidElements { t.Fatalf("solve e=1: %v", err) }
	s, err := store.Open(filepath.Join(t.TempDir(), "orbitops.db")); if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = s.Close() })
	if _, err := service.New(s, clock.NewFake(1)).RegisterSatellite(context.Background(), "bad", "bad", el); err != model.ErrInvalidElements { t.Fatalf("register e=1: %v", err) }
}
