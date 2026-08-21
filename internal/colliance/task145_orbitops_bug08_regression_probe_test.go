package colliance_test

import (
	"testing"

	"orbitops/internal/colliance"
	"orbitops/internal/maneuver"
	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

func TestBug08_ExactCollisionSafetyMarginIsAccepted(t *testing.T) {
	if orbmath.CollisionSafeKm != 20 { t.Fatalf("published safety margin=%v", orbmath.CollisionSafeKm) }
	p := colliance.New(propagator.New()); _, post, err := p.PlanAvoidance(model.Elements{},model.Elements{},&colliance.EvalResult{TCA:100,MinDistanceKm:10},0)
	if err != nil || post != 20 { t.Fatalf("avoidance post margin=%v err=%v",post,err) }
	if _, err := maneuver.PlanCollisionAvoidance("s","a",1,20,10,model.Elements{},0); err != nil { t.Fatalf("exact safe maneuver rejected: %v",err) }
}
