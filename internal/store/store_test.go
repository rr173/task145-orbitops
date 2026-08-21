package store

import (
	"context"
	"path/filepath"
	"testing"

	"orbitops/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "orbitops_test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSatelliteCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sat := model.Satellite{
		ID: "sat-1", Name: "TestSat", Catalog: "25544",
		Elements:  model.Elements{A: 7000, E: 0.001, I: 51.6, Raan: 0, Argp: 0, M: 0, Epoch: 1000},
		Status:    model.StatusNominal,
		CreatedAt: 1000, UpdatedAt: 1000,
	}
	if err := s.InTx(ctx, func(q *Queries) error {
		return q.InsertSatellite(ctx, sat)
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.q.GetSatellite(ctx, "sat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Elements.A != 7000 || got.Status != model.StatusNominal {
		t.Fatalf("unexpected: %+v", got)
	}
	lst, err := s.q.ListSatellites(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(lst) != 1 {
		t.Fatalf("list len=%d", len(lst))
	}
}

func TestElementHistoryLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.InTx(ctx, func(q *Queries) error { return q.InsertSatellite(ctx, model.Satellite{ID: "s1", Name: "n", Catalog: "c", Elements: model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}, Status: model.StatusNominal, CreatedAt: 0, UpdatedAt: 0}) })
	for i, ep := range []model.Epoch{1000, 2000, 1500} {
		h := model.ElementHistory{ID: "", SatelliteID: "s1", Elements: model.Elements{A: float64(7000 + i), E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: ep}, Source: model.SourceTLEUpdate, CreatedAt: ep}
		s.InTx(ctx, func(q *Queries) error { return q.InsertElementHistory(ctx, h) })
	}
	latest, err := s.q.LatestElementHistory(ctx, "s1")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Elements.Epoch != 2000 {
		t.Fatalf("latest epoch = %d want 2000", latest.Elements.Epoch)
	}
}

func TestEventOrdering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, ts := range []int64{3000, 1000, 2000} {
		e := model.Event{Type: model.EventTLEUpdate, SatelliteID: "s1", PayloadJSON: "{}", Ts: model.Epoch(ts), CreatedAt: model.Epoch(ts)}
		s.InTx(ctx, func(q *Queries) error { return q.AppendEvent(ctx, e) })
	}
	evs, err := s.q.ListEventsOrdered(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("len=%d", len(evs))
	}
	if evs[0].Ts != 1000 || evs[1].Ts != 2000 || evs[2].Ts != 3000 {
		t.Fatalf("order wrong: %+v", evs)
	}
}

func TestNextWindowUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	nw := model.NextWindow{SatelliteID: "s1", NextContactEpoch: 5000, NextContactStation: "sta", OpenAlerts: 2}
	s.InTx(ctx, func(q *Queries) error { return q.UpsertNextWindow(ctx, nw) })
	nw.OpenAlerts = 3
	s.InTx(ctx, func(q *Queries) error { return q.UpsertNextWindow(ctx, nw) })
	got, err := s.q.GetNextWindow(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OpenAlerts != 3 {
		t.Fatalf("open alerts = %d want 3", got.OpenAlerts)
	}
}
