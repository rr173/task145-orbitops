package orbmath

import (
	"math"
	"testing"

	"orbitops/internal/model"
)

func TestSolveKeplerCircular(t *testing.T) {
	// circular orbit e=0: E must equal M exactly.
	E, err := SolveKepler(0.5, 0.0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if absFloat(E-0.5) > 1e-9 {
		t.Fatalf("E=%v want 0.5", E)
	}
}

func TestSolveKeplerModerate(t *testing.T) {
	// e=0.5, M=1.0 rad. Solve and verify M = E - e sin E.
	E, err := SolveKepler(1.0, 0.5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	M := E - 0.5*sinFloat(E)
	if absFloat(M-1.0) > 1e-9 {
		t.Fatalf("residual M=%v want 1.0", M)
	}
}

func TestSolveKeplerRejectsE(t *testing.T) {
	if _, err := SolveKepler(0.5, 1.0); err == nil {
		t.Fatal("expected error for e>=1")
	}
	if _, err := SolveKepler(0.5, -0.1); err == nil {
		t.Fatal("expected error for e<0")
	}
}

func TestMeanMotion(t *testing.T) {
	// GEO satellite: period should be one sidereal day (~86164 s).
	n := MeanMotion(AGeo)
	period := TwoPi / n
	if absFloat(period-86164.0905) > 1.0 {
		t.Fatalf("GEO period=%v want ~86164s", period)
	}
}

func TestGMSTMonotonic(t *testing.T) {
	a := GMST(0)
	b := GMST(86400) // one solar day later: advances ~360.9856deg => ~0.9856deg residual mod 360
	// sidereal day (86164s) ~ solar day minus ~4min/day, so residual ~0.0172 rad.
	if absFloat(normalizeRadPi(a)-normalizeRadPi(b)) > 0.02 {
		t.Fatalf("GMST residual too large: a=%v b=%v", a, b)
	}
}

func TestECEFLLARoundTrip(t *testing.T) {
	lla := model.LatLonAlt{Lat: 40.0, Lon: -75.0, Alt: 0.0}
	ecef := LLAtoECEF(lla.Lat, lla.Lon, lla.Alt)
	back := ECEFtoLLA(ecef)
	if absFloat(back.Lat-lla.Lat) > 1e-6 || absFloat(NormalizeDeg(back.Lon)-NormalizeDeg(lla.Lon)) > 1e-6 {
		t.Fatalf("round-trip failed: got %+v want %+v", back, lla)
	}
}

// TestNormalizeDegCanonicalAntimeridian locks the canonical representation
// across the international date line: +180 and -180 denote the same meridian
// and must both collapse to -180, and the function must be idempotent so a
// value already in the range is never shifted.
func TestNormalizeDegCanonicalAntimeridian(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{180, -180},   // +180 folds to -180 (single canonical antimeridian)
		{-180, -180},
		{540, -180},   // 540 -> 180 -> -180
		{-540, -180},
		{179.999, 179.999},
		{-179.999, -179.999},
		{360, 0},
		{-360, 0},
	}
	for _, c := range cases {
		got := NormalizeDeg(c.in)
		if got != c.want {
			t.Errorf("NormalizeDeg(%v) = %v, want %v", c.in, got, c.want)
		}
		// idempotency: normalizing an already-normalized value is a no-op
		if n2 := NormalizeDeg(got); n2 != got {
			t.Errorf("NormalizeDeg not idempotent at %v: got %v", got, n2)
		}
		// invariant: result lives in [-180,180)
		if got < -180 || got >= 180 {
			t.Errorf("NormalizeDeg(%v) = %v outside [-180,180)", c.in, got)
		}
	}
	// +180 and -180 must produce the SAME canonical value (same place, one number)
	if NormalizeDeg(180) != NormalizeDeg(-180) {
		t.Fatal("+180 and -180 must canonicalize to the same value")
	}
}

// TestECEFtoLLAAntimeridianCanonical ensures the sub-satellite longitude
// never leaks +180 at the date line: a point on the antimeridian (negative X
// axis, equator) reports -180, matching the station-longitude representation
// so the same position is never stored as two different numbers.
func TestECEFtoLLAAntimeridianCanonical(t *testing.T) {
	// ECEF point on the antimeridian at the equator: (-Req, 0, 0).
	lla := ECEFtoLLA([3]float64{-Req, 0, 0})
	if lla.Lon != -180 {
		t.Fatalf("ECEFtoLLA(-Req,0,0).Lon = %v, want -180 (canonical)", lla.Lon)
	}
	if lla.Lat < -0.001 || lla.Lat > 0.001 {
		t.Fatalf("ECEFtoLLA(-Req,0,0).Lat = %v, want ~0", lla.Lat)
	}
	// +180 and -180 ground stations map to the same ECEF round-trip longitude.
	p := LLAtoECEF(0, 180, 0)
	n := LLAtoECEF(0, -180, 0)
	if ECEFtoLLA(p).Lon != ECEFtoLLA(n).Lon {
		t.Fatal("lon=+180 and lon=-180 must round-trip to the same canonical longitude")
	}
}

func TestPerigeeAltitude(t *testing.T) {
	// a=7000, e=0.1: perigee altitude = 7000*0.9 - 6378.137 = 6300-6378.137 = -78.137 (suborbital-ish)
	hp := PerigeeAltitudeKm(7000.0, 0.1)
	if absFloat(hp-(-78.137)) > 0.01 {
		t.Fatalf("hp=%v want -78.137", hp)
	}
}

func TestJ2RAANRegression(t *testing.T) {
	// Sun-synchronous-like: retrograde drift should be positive (eastward) for i>90.
	raanDot, _, _ := J2SecularRates(7100, 0.001, 98.0)
	if raanDot <= 0 {
		t.Fatalf("expected positive RAAN drift for retrograde orbit, got %v", raanDot)
	}
}

func TestInEarthShadowBasic(t *testing.T) {
	// Sun along +X; satellite at -X behind earth within Req distance -> shadow.
	sun := [3]float64{1, 0, 0}
	sat := [3]float64{-7000, 0, 0}
	if !InEarthShadow(sat, sun) {
		t.Fatal("expected satellite behind earth in shadow")
	}
	sat2 := [3]float64{7000, 0, 0}
	if InEarthShadow(sat2, sun) {
		t.Fatal("satellite on sun side should not be in shadow")
	}
}

func TestIsGEO(t *testing.T) {
	if !IsGEO(AGeo, 0.05) {
		t.Fatal("AGeo+small incl should be GEO")
	}
	if IsGEO(7000, 45) {
		t.Fatal("LEO should not be GEO")
	}
}

// local helpers avoiding importing math elsewhere at test time
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func sinFloat(v float64) float64 { return math.Sin(v) }
func sinHelper(v float64) float64 { return math.Sin(v) }
func normalizeRadPi(r float64) float64 {
	for r > math.Pi {
		r -= 2 * math.Pi
	}
	for r < -math.Pi {
		r += 2 * math.Pi
	}
	return r
}
