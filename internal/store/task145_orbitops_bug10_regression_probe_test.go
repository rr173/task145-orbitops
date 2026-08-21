package store

import (
	"context"
	"testing"

	"orbitops/internal/model"
)

func TestBug10_PersistedLifecycleStatesRoundTrip(t *testing.T) {
	s := newTestStore(t); ctx := context.Background()
	if err := s.InTx(ctx, func(q *Queries) error {
		if err := q.InsertContact(ctx,model.Contact{ID:"c",SatelliteID:"s",StationID:"st",AOS:1,Los:2,TCA:1,Source:model.ContactReplay}); err != nil{return err}
		if err := q.InsertAlert(ctx,model.CollisionAlert{ID:"a",PrimaryID:"s",SecondaryID:"t",Status:model.AlertAvoided}); err != nil{return err}
		return q.InsertManeuver(ctx,model.Maneuver{ID:"m",SatelliteID:"s",Type:model.ManeuverPerigeeRaise,Status:model.ManeuverExecuting})
	}); err != nil {t.Fatal(err)}
	c,err:=s.GetContact(ctx,"c"); if err!=nil||c.Source!=model.ContactReplay {t.Fatalf("contact=%+v err=%v",c,err)}
	a,err:=s.GetAlert(ctx,"a"); if err!=nil||a.Status!=model.AlertAvoided {t.Fatalf("alert=%+v err=%v",a,err)}
	m,err:=s.GetManeuver(ctx,"m"); if err!=nil||m.Status!=model.ManeuverExecuting {t.Fatalf("maneuver=%+v err=%v",m,err)}
}
