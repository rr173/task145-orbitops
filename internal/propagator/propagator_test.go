package propagator

import (
	"math"
	"testing"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
)

func mustState(t *testing.T, p *Propagator, el model.Elements, at model.Epoch) model.StateVector {
	t.Helper()
	sv, err := p.StateAt(el, at)
	if err != nil {
		t.Fatalf("StateAt err: %v", err)
	}
	return sv
}

func TestStateAtCircular(t *testing.T) {
	p := New()
	// circular LEO at 7000 km altitude from center => a = 7000+6378=13378? No:
	// a is from center, so a=7000 km is a sub-orbital-ish. Use a=7000.
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	sv := mustState(t, p, el, 0)
	// at M=0, E=0, perifocal pos = (a(cos0 - e), 0) = (a, 0); with Omega=i=omega=0
	// ECI pos = (a, 0, 0).
	if math.Abs(sv.X-7000) > 1e-6 || math.Abs(sv.Y) > 1e-6 || math.Abs(sv.Z) > 1e-6 {
		t.Fatalf("expected (7000,0,0) got %+v", sv)
	}
}

func TestStateAtAdvanceHalfOrbit(t *testing.T) {
	p := New()
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	// period T = 2π/n; half period later satellite on opposite side.
	n := math.Sqrt(398600.4418 / (7000 * 7000 * 7000))
	period := 2 * math.Pi / n
	sv := mustState(t, p, el, model.Epoch(period/2))
	// expect pos near (-7000, 0, 0); with i=0 the J2 argp drift is 0 but the
	// mean-anomaly correction mDot is nonzero (advances the satellite slightly),
	// producing a small +Y offset on the order of tens of km. Allow 50 km.
	if math.Abs(sv.X+7000) > 1.0 {
		t.Fatalf("X expected near -7000 got %v", sv.X)
	}
	if math.Abs(sv.Y) > 50.0 {
		t.Fatalf("Y drift too large: got %v", sv.Y)
	}
}

func TestSubPointAltitude(t *testing.T) {
	p := New()
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	lla, _, err := p.SubPoint(el, 0)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	// circular a=7000 -> altitude ~7000-6378.137 = 621.86 km
	if math.Abs(lla.Alt-621.863) > 0.5 {
		t.Fatalf("alt=%v want ~621.86", lla.Alt)
	}
}

func TestValidate(t *testing.T) {
	if Validate(model.Elements{A: 7000, E: 1.2}) == nil {
		t.Fatal("e>=1 should fail")
	}
	if Validate(model.Elements{A: -1, E: 0.1}) == nil {
		t.Fatal("a<=0 should fail")
	}
	if Validate(model.Elements{A: 7000, E: 0.1}) != nil {
		t.Fatal("valid elements should pass")
	}
}

func TestRangeBetweenSameSat(t *testing.T) {
	p := New()
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	d, err := p.RangeBetween(el, el, 0)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if d > 1e-6 {
		t.Fatalf("range between same sat should be ~0 got %v", d)
	}
}

// TestAdvancedElementsAnglesCanonicalRange ensures all propagated angular
// elements (M, Raan, Argp) stay in the canonical [-180,180) range so the
// element set never accumulates unbounded or out-of-range angles across many
// propagation steps — a uniform representation regardless of date-line crossing.
func TestAdvancedElementsAnglesCanonicalRange(t *testing.T) {
	p := New()
	// GEO with J2 secular drift advancing over a long horizon.
	el := model.Elements{A: orbmath.AGeo, E: 0.001, I: 0.1, Raan: 170, Argp: 0, M: 0, Epoch: 0}
	adv, err := p.AdvancedElements(el, 2000000) // ~23 days
	if err != nil {
		t.Fatalf("AdvancedElements err: %v", err)
	}
	for _, a := range []float64{adv.M, adv.Raan, adv.Argp} {
		if a < -180 || a >= 180 {
			t.Errorf("angle %v outside canonical [-180,180)", a)
		}
	}
}

