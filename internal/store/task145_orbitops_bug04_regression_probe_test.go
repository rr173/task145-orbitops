package store

import (
	"context"
	"testing"
)

func TestBug04_CanceledRequestStopsEveryStorePath(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.InTx(ctx, func(*Queries) error { return nil }); err == nil { t.Fatal("canceled transaction was started") }
	if _, err := s.ListContacts(ctx, "", ""); err == nil { t.Fatal("canceled contact list returned normally") }
	if _, err := s.ListEventsOrdered(ctx); err == nil { t.Fatal("canceled event list returned normally") }
}
