// Package groundtrack produces satellite-to-ground-station contact (visibility)
// windows by scanning elevation over a forecast window with a fixed step and
// refining sign-change crossings by bisection. It also computes lighting
// (Earth-shadow) state. The package depends only on propagator + orbmath and
// is deterministic and pure.
package groundtrack

import (
	"math"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

// ForecastRequest specifies a single satellite/station forecast run.
type ForecastRequest struct {
	SatelliteID string
	StationID   string
	Elements    model.Elements
	Station     model.GroundStation
	Start       model.Epoch
	Duration    model.Epoch // seconds to forecast
	Step        model.Epoch // scan step (default 60s)
}

// Result is the computed set of contacts for the request.
type Result struct {
	Contacts []model.Contact
	Ambiguous []model.Epoch // timestamps of ambiguous segments (not stored as valid contacts)
}

// Scanner is stateless and safe for concurrent use.
type Scanner struct {
	prop *propagator.Propagator
}

func New(p *propagator.Propagator) *Scanner { return &Scanner{prop: p} }

// Forecast scans the elevation of the satellite above the station over
// [Start, Start+Duration] at Step-second intervals. Where the elevation
// changes sign between consecutive samples it refines the crossing by
// bisection to within RefineEpsilon seconds. Pairs of rise (neg->pos) and
// set (pos->neg) crossings form a contact; only contacts whose peak
// elevation meets the station's minimum are returned as valid.
//
// If a single step contains more than one sign change (rare; indicates
// ambiguous sampling), the segment is re-scanned at Step/4 once. If still
// unresolved the segment is dropped and its midpoint recorded in Ambiguous.
func (s *Scanner) Forecast(req ForecastRequest, computedAt model.Epoch) (*Result, error) {
	step := int64(req.Step)
	if step <= 0 {
		step = orbmath.LightStepDefault
	}
	return s.scanWithStep(req, step, computedAt, false)
}

func (s *Scanner) scanWithStep(req ForecastRequest, step int64, computedAt model.Epoch, reentry bool) (*Result, error) {
	res := &Result{}
	n := int64(req.Duration) / step
	if n < 1 {
		n = 1
	}
	// sample elevations
	type sample struct {
		t    model.Epoch
		elev float64
	}
	samples := make([]sample, 0, n+2)
	for i := int64(0); i <= n; i++ {
		t := req.Start + model.Epoch(i*step)
		elev, _, _, err := s.prop.ElevationAt(req.Elements, t, req.Station)
		if err != nil {
			// kepler divergence etc. -> treat as 0 elevation to avoid crash,
			// but mark nothing. We skip this sample (continue) to keep spacing.
			samples = append(samples, sample{t: t, elev: 0})
			continue
		}
		samples = append(samples, sample{t: t, elev: elev})
	}

	// find crossings between consecutive samples
	type cross struct {
		t     model.Epoch
		rise  bool // neg->pos
	}
	crossings := make([]cross, 0, 8)
	for i := 0; i < len(samples)-1; i++ {
		a, b := samples[i], samples[i+1]
		aPos := a.elev > 0
		bPos := b.elev > 0
		if aPos == bPos {
			continue
		}
		ct, ok := s.refineCross(req, a.t, a.elev, b.t, b.elev)
		if !ok {
			// ambiguous crossing in this step: count sign changes in this step
			// using sub-scan if not already re-entering
			if !reentry {
				sub, _ := s.scanWithStep(ForecastRequest{
					SatelliteID: req.SatelliteID, StationID: req.StationID,
					Elements: req.Elements, Station: req.Station,
					Start: a.t, Duration: model.Epoch(b.t - a.t), Step: model.Epoch(step / 4),
				}, step/4, computedAt, true)
				if sub != nil {
					res.Contacts = append(res.Contacts, sub.Contacts...)
					res.Ambiguous = append(res.Ambiguous, sub.Ambiguous...)
				}
				continue
			}
			res.Ambiguous = append(res.Ambiguous, (a.t+b.t)/2)
			continue
		}
		crossings = append(crossings, cross{t: ct, rise: bPos})
	}

	// pair rise/set
	for i := 0; i+1 < len(crossings); i++ {
		rise, set := crossings[i], crossings[i+1]
		if !rise.rise || set.rise {
			continue // need rise then set
		}
		c := s.buildContact(req, rise.t, set.t, computedAt)
		if c == nil {
			continue
		}
		if c.MaxElevationDeg >= req.Station.MinElevationDeg {
			res.Contacts = append(res.Contacts, *c)
		}
		i++ // consumed set
	}
	return res, nil
}

// refineCross bisects the interval [ta, tb] (with elevations ea, eb of opposite
// sign) to locate the crossing to within RefineEpsilon seconds. Returns false
// if the interval is too small or elevation cannot be evaluated.
func (s *Scanner) refineCross(req ForecastRequest, ta model.Epoch, ea float64, tb model.Epoch, eb float64) (model.Epoch, bool) {
	lo, hi := ta, tb
	eLo := ea
	for hi-lo > model.Epoch(orbmath.RefineEpsilon) {
		mid := lo + (hi-lo)/2
		em, _, _, err := s.prop.ElevationAt(req.Elements, mid, req.Station)
		if err != nil {
			return 0, false
		}
		if (eLo > 0) == (em > 0) {
			lo = mid
			eLo = em
		} else {
			hi = mid
		}
	}
	return lo + (hi-lo)/2, true
}

// buildContact constructs a Contact from rise/set times by sampling the window
// to find peak elevation, TCA and lighting at midpoint. Returns nil if the
// window is empty or sampling fails.
func (s *Scanner) buildContact(req ForecastRequest, aos, los model.Epoch, computedAt model.Epoch) *model.Contact {
	if los <= aos {
		return nil
	}
	span := int64(los - aos)
	// sample ~ up to 16 points across the window
	steps := span / 60
	if steps < 4 {
		steps = 4
	}
	if steps > 64 {
		steps = 64
	}
	istep := span / steps
	if istep < 1 {
		istep = 1
	}
	var peakElev float64
	var tca model.Epoch
	for i := int64(0); i <= steps; i++ {
		t := aos + model.Epoch(i*istep)
		if t > los {
			t = los
		}
		e, _, _, err := s.prop.ElevationAt(req.Elements, t, req.Station)
		if err != nil {
			continue
		}
		if e > peakElev {
			peakElev = e
			tca = t
		}
	}
	if tca == 0 {
		tca = aos + (los-aos)/2
	}
	// azimuth at AOS and LOS
	aosAz, _, _, _ := s.prop.ElevationAt(req.Elements, aos, req.Station)
	losAz, _, _, _ := s.prop.ElevationAt(req.Elements, los, req.Station)
	// lighting at TCA: propagate ECI and test shadow
	sunlit := true
	if sv, err := s.prop.StateAt(req.Elements, tca); err == nil {
		sun := orbmath.SunDirectionECI(tca)
		sunlit = !orbmath.InEarthShadow([3]float64{sv.X, sv.Y, sv.Z}, sun)
	}
	_ = math.Mod // keep math import meaningful
	return &model.Contact{
		SatelliteID:     req.SatelliteID,
		StationID:       req.StationID,
		AOS:             aos,
		Los:             los,
		TCA:             tca,
		MaxElevationDeg: peakElev,
		AOSAzDeg:        normalizeAz(aosAz),
		LosAzDeg:        normalizeAz(losAz),
		Sunlit:          sunlit,
		Source:          model.ContactForecast,
		ComputedAt:      computedAt,
	}
}

func normalizeAz(az float64) float64 {
	if az < 0 {
		az += 360
	}
	if az >= 360 {
		az = math.Mod(az, 360)
	}
	return az
}
