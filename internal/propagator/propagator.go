// Package propagator advances a satellite's element set to a target epoch and
// returns its ECI state vector and geodetic sub-satellite point. It builds on
// orbmath: two-body Kepler propagation plus first-order J2 secular drift and
// (informational) drag-induced perigee decay. The propagator is pure and
// deterministic — same inputs always yield the same state.
package propagator

import (
	"math"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
)

// Propagator is stateless; methods are safe for concurrent use.
type Propagator struct{}

func New() *Propagator { return &Propagator{} }

// Validate returns ErrInvalidElements when e<0, e>=1 or a<=0.
func Validate(e model.Elements) error {
	if e.E < 0 || e.E >= 1 || e.A <= 0 {
		return model.ErrInvalidElements
	}
	return nil
}

// AdvancedElements returns the element set advanced to epoch t by:
//   - mean anomaly advance via mean motion n = sqrt(mu/a^3)
//   - J2 secular drift of RAAN, argument of perigee and mean-anomaly correction
// It does NOT mutate the input. Angles returned are in degrees (consistent
// with the stored form); internally radians are used.
func (p *Propagator) AdvancedElements(el model.Elements, t model.Epoch) (model.Elements, error) {
	if err := Validate(el); err != nil {
		return model.Elements{}, err
	}
	dt := float64(t - el.Epoch)
	n := orbmath.MeanMotion(el.A)
	raanDot, argpDot, mDot := orbmath.J2SecularRates(el.A, el.E, el.I)

	adv := el
	adv.Epoch = t
	// advance angles (radians) then convert back to degrees
	M := orbmath.D2R(el.M) + n*dt + mDot*dt
	adv.M = orbmath.R2D(orbmath.NormalizeRad(M))
	adv.Raan = orbmath.NormalizeDeg(el.Raan + orbmath.R2D(raanDot)*dt)
	adv.Argp = orbmath.NormalizeDeg(el.Argp + orbmath.R2D(argpDot)*dt)
	return adv, nil
}

// StateAt returns the ECI state vector (km, km/s) at epoch t. It advances the
// elements to t, solves Kepler's equation for the eccentric anomaly, forms the
// perifocal position/velocity, and rotates into ECI.
func (p *Propagator) StateAt(el model.Elements, t model.Epoch) (model.StateVector, error) {
	adv, err := p.AdvancedElements(el, t)
	if err != nil {
		return model.StateVector{}, err
	}
	M := orbmath.D2R(adv.M)
	E, err := orbmath.SolveKepler(M, adv.E)
	if err != nil {
		return model.StateVector{}, err
	}
	rx, ry, r := orbmath.PerifocalPosition(adv.A, adv.E, E)
	vx, vy, _ := orbmath.PerifocalVelocity(adv.A, adv.E, E, r)
	_ = r

	pos := orbmath.Rotate3([3]float64{rx, ry, 0}, orbmath.D2R(adv.Raan), orbmath.D2R(adv.I), orbmath.D2R(adv.Argp))
	vel := orbmath.Rotate3([3]float64{vx, vy, 0}, orbmath.D2R(adv.Raan), orbmath.D2R(adv.I), orbmath.D2R(adv.Argp))
	return model.StateVector{
		X: pos[0], Y: pos[1], Z: pos[2],
		VX: vel[0], VY: vel[1], VZ: vel[2],
	}, nil
}

// SubPoint returns the geodetic sub-satellite point at epoch t: ECI -> ECEF
// (rotated by GMST at t) -> lat/lon/alt.
func (p *Propagator) SubPoint(el model.Elements, t model.Epoch) (model.LatLonAlt, model.StateVector, error) {
	sv, err := p.StateAt(el, t)
	if err != nil {
		return model.LatLonAlt{}, sv, err
	}
	gmst := orbmath.GMST(t)
	ecef := orbmath.ECItoECEF([3]float64{sv.X, sv.Y, sv.Z}, gmst)
	lla := orbmath.ECEFtoLLA(ecef)
	return lla, sv, nil
}

// ElevationAt returns the elevation (degrees) of the satellite above a ground
// station at epoch t, along with azimuth (degrees) and slant range (km).
func (p *Propagator) ElevationAt(el model.Elements, t model.Epoch, st model.GroundStation) (elevDeg, azDeg, rangeKm float64, err error) {
	sv, err := p.StateAt(el, t)
	if err != nil {
		return 0, 0, 0, err
	}
	gmst := orbmath.GMST(t)
	satECEF := orbmath.ECItoECEF([3]float64{sv.X, sv.Y, sv.Z}, gmst)
	staECEF := orbmath.LLAtoECEF(st.LatDeg, st.LonDeg, st.AltM*orbmath.KmPerM)
	_, _, _, rng, elev, az := orbmath.RelativeENU(satECEF, staECEF, st.LatDeg, st.LonDeg)
	return elev, az, rng, nil
}

// DriftRateDegPerDay estimates the east-west longitude drift rate of a GEO
// satellite by differencing the sub-satellite longitude over one day. Returns
// degrees/day (positive = eastward drift).
func (p *Propagator) DriftRateDegPerDay(el model.Elements) (float64, error) {
	if !orbmath.IsGEO(el.A, el.I) {
		return 0, nil
	}
	now := el.Epoch
	lla0, _, err := p.SubPoint(el, now)
	if err != nil {
		return 0, err
	}
	lla1, _, err := p.SubPoint(el, now+86400)
	if err != nil {
		return 0, err
	}
	dLon := orbmath.NormalizeDeg(lla1.Lon - lla0.Lon)
	return dLon, nil
}

// PerigeeAfterDrag returns the predicted perigee altitude (km) after `days`
// of drag-induced decay, using the locked per-revolution decay estimate. It is
// informational: the main propagation path uses the latest semi-major axis
// from element history; this predicts when perigee_raise should trigger.
func (p *Propagator) PerigeeAfterDrag(el model.Elements, B float64, days float64) float64 {
	if B <= 0 {
		B = orbmath.DefaultBallisticCoef
	}
	hp := orbmath.PerigeeAltitudeKm(el.A, el.E)
	periodSec := orbmath.TwoPi / orbmath.MeanMotion(el.A)
	revsPerDay := 86400.0 / periodSec
	decayPerRev := orbmath.DragPerigeeDecayKmPerRev(B, el.A, hp)
	return hp + decayPerRev*revsPerDay*days
}

// RangeBetween returns the Euclidean distance (km) between two satellites at
// epoch t, each propagated from its own elements.
func (p *Propagator) RangeBetween(a, b model.Elements, t model.Epoch) (float64, error) {
	sa, err := p.StateAt(a, t)
	if err != nil {
		return 0, err
	}
	sb, err := p.StateAt(b, t)
	if err != nil {
		return 0, err
	}
	dx := sa.X - sb.X
	dy := sa.Y - sb.Y
	dz := sa.Z - sb.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz), nil
}
