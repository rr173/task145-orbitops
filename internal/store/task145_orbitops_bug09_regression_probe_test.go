package store

import (
	"context"
	"testing"
)

func TestBug09_EmptyCollectionsRemainAbsent(t *testing.T) {
	s := newTestStore(t); ctx := context.Background()
	contacts, err := s.ListContacts(ctx,"",""); if err != nil || contacts != nil { t.Fatalf("empty contacts=%#v err=%v",contacts,err) }
	alerts, err := s.ListAlerts(ctx,""); if err != nil || alerts != nil { t.Fatalf("empty alerts=%#v err=%v",alerts,err) }
	maneuvers, err := s.ListManeuvers(ctx,""); if err != nil || maneuvers != nil { t.Fatalf("empty maneuvers=%#v err=%v",maneuvers,err) }
}
