package groundtrack

import (
	"testing"

	"orbitops/internal/model"
	"orbitops/internal/propagator"
)

func TestForecastProducesPass(t *testing.T) {
	p := propagator.New()
	sc := New(p)
	// LEO satellite at a=7000km, i=0, station near equator should see passes
	// during a half-day window. Period ~ 5780s so multiple passes in 12h.
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	st := model.GroundStation{
		ID: "sta1", Name: "Equator", LatDeg: 0, LonDeg: 0, AltM: 0, MinElevationDeg: 5,
	}
	res, err := sc.Forecast(ForecastRequest{
		SatelliteID: "s1", StationID: "sta1", Elements: el, Station: st,
		Start: 0, Duration: 12 * 3600, Step: 60,
	}, 0)
	if err != nil {
		t.Fatalf("Forecast err: %v", err)
	}
	if len(res.Contacts) == 0 {
		t.Fatalf("expected at least one pass over 12h, got 0")
	}
	for _, c := range res.Contacts {
		if c.Los <= c.AOS {
			t.Fatalf("contact los<=aos: %+v", c)
		}
		if c.MaxElevationDeg < 5 {
			t.Fatalf("pass below threshold: %+v", c)
		}
	}
}

func TestForecastNoPassWhenFarPolar(t *testing.T) {
	// Equatorial satellite should not rise above a high-latitude polar station
	// at the instant the satellite is over the equator at lon 0. Over a short
	// window (one orbit) the peak elevation stays low.
	p := propagator.New()
	sc := New(p)
	el := model.Elements{A: 7000, E: 0, I: 0, Raan: 0, Argp: 0, M: 0, Epoch: 0}
	st := model.GroundStation{
		ID: "pole", Name: "NorthPole", LatDeg: 85, LonDeg: 0, AltM: 0, MinElevationDeg: 5,
	}
	res, err := sc.Forecast(ForecastRequest{
		SatelliteID: "s1", StationID: "pole", Elements: el, Station: st,
		Start: 0, Duration: 6000, Step: 30,
	}, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.Contacts) > 0 {
		t.Fatalf("polar station should not see equatorial sat in one orbit, got %d passes", len(res.Contacts))
	}
}
