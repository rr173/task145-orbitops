package store

import (
	"context"
	"testing"

	"orbitops/internal/model"
)

func TestBug03_SameEpochRecordsKeepCreationOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.InTx(ctx, func(q *Queries) error {
		if err := q.AppendEvent(ctx, model.Event{ID: "early", Type: model.EventTLEUpdate, SatelliteID: "s", PayloadJSON: "{}", Ts: 100, CreatedAt: 10}); err != nil { return err }
		return q.AppendEvent(ctx, model.Event{ID: "late", Type: model.EventManeuver, SatelliteID: "s", PayloadJSON: "{}", Ts: 100, CreatedAt: 20})
	}); err != nil { t.Fatal(err) }
	events, err := s.ListEventsOrdered(ctx)
	if err != nil || len(events) != 2 || events[0].ID != "early" || events[1].ID != "late" { t.Fatalf("same-epoch event order=%+v err=%v", events, err) }
	if err := s.InTx(ctx, func(q *Queries) error {
		if err := q.InsertSatellite(ctx, model.Satellite{ID: "s", Name: "s", Catalog: "s", Elements: model.Elements{A: 7000}, Status: model.StatusNominal}); err != nil { return err }
		if err := q.InsertElementHistory(ctx, model.ElementHistory{ID: "first", SatelliteID: "s", Elements: model.Elements{A: 7000, Epoch: 100}, Source: model.SourceInitial, CreatedAt: 10}); err != nil { return err }
		return q.InsertElementHistory(ctx, model.ElementHistory{ID: "second", SatelliteID: "s", Elements: model.Elements{A: 7001, Epoch: 100}, Source: model.SourceTLEUpdate, CreatedAt: 20})
	}); err != nil { t.Fatal(err) }
	history, err := s.ListElementHistory(ctx, "s")
	if err != nil || history[0].ID != "first" || history[1].ID != "second" { t.Fatalf("same-epoch history order=%+v err=%v", history, err) }
}
