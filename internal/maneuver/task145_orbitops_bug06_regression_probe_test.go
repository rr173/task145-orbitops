package maneuver_test

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/clock"
	"orbitops/internal/maneuver"
	"orbitops/internal/model"
	"orbitops/internal/service"
	"orbitops/internal/store"
)

func TestBug06_ActiveManeuverWindowsShareOneBoundaryRule(t *testing.T) {
	if maneuver.Overlap(maneuver.Window{Start: 10, End: 11}, maneuver.Window{Start: 11, End: 12}) { t.Fatal("touching one-second windows must not overlap") }
	s, err := store.Open(filepath.Join(t.TempDir(), "orbitops.db")); if err != nil { t.Fatal(err) }; t.Cleanup(func(){ _ = s.Close() })
	ctx := context.Background()
	if err := s.InTx(ctx, func(q *store.Queries) error { return q.InsertManeuver(ctx, model.Maneuver{ID:"m", SatelliteID:"s", Type:model.ManeuverPerigeeRaise, ExecutedAt:100, Status:model.ManeuverPlanned}) }); err != nil { t.Fatal(err) }
	active, err := s.Q().ListActiveManeuvers(ctx, "s", 100, 100); if err != nil || len(active) != 1 { t.Fatalf("boundary active set=%+v err=%v", active, err) }
	svc := service.New(s, clock.NewFake(100)); sat, err := svc.RegisterSatellite(ctx,"low","l",model.Elements{A:6600,E:0.12,I:28,Epoch:100}); if err != nil { t.Fatal(err) }
	if _, err = svc.PlanManeuver(ctx,sat.ID,model.ManeuverPerigeeRaise,100); err != nil { t.Fatal(err) }
	if _, err = svc.PlanManeuver(ctx,sat.ID,model.ManeuverPerigeeRaise,100); err != model.ErrManeuverConflict { t.Fatalf("same execution window must conflict, got %v",err) }
}
