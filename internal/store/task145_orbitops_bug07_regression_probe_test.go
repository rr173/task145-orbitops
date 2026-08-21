package store

import (
	"context"
	"testing"

	"orbitops/internal/model"
)

func TestBug07_NextWindowKeepsExactRecoveryBoundary(t *testing.T) {
	s := newTestStore(t); ctx := context.Background()
	if err := s.InTx(ctx, func(q *Queries) error { return q.InsertContact(ctx, model.Contact{ID:"c",SatelliteID:"sat",StationID:"st",AOS:100,Los:110,TCA:105}) }); err != nil { t.Fatal(err) }
	c, err := s.Q().NextContactAfter(ctx,"sat",100); if err != nil || c.ID != "c" { t.Fatalf("contact at recovery boundary=%+v err=%v",c,err) }
	if err := s.InTx(ctx, func(q *Queries) error { return q.UpsertNextWindow(ctx,model.NextWindow{SatelliteID:"sat",NextContactEpoch:100,OpenAlerts:2}) }); err != nil { t.Fatal(err) }
	nw, err := s.GetNextWindow(ctx,"sat"); if err != nil || nw.OpenAlerts != 2 { t.Fatalf("next-window cache=%+v err=%v",nw,err) }
}
