// Package maneuver plans and validates propulsive burns: east-west
// stationkeeping for GEO satellites, perigee-raising for LEO satellites whose
// drag-degraded perigee falls below threshold, and north-south inclination
// maneuvers. Collision-avoidance burns are also produced here but driven by
// the colliance package's inputs.
//
// The locked policy constants (deadband, lead-time, gains, thresholds) live
// in orbmath; this package encodes the decision rules and conflict checks.
package maneuver

import (
	"math"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

// Planner is stateless; safe for concurrent use.
type Planner struct {
	prop *propagator.Propagator
}

func New(p *propagator.Propagator) *Planner { return &Planner{prop: p} }

// EWDecision captures the decision to (not) perform an east-west
// stationkeeping burn. Drift is degrees/day (positive = eastward). When
// |drift| exceeds the deadband, the burn is required and the latest acceptable
// execution time is the crossing time + EWLeadTimeDays.
type EWDecision struct {
	Required       bool
	Drift          float64 // deg/day
	CrossingAt     model.Epoch
	LatestExecAt   model.Epoch
	DeltaVMps      float64
	TargetSMAKm    float64 // corrected semi-major axis
}

// EvaluateEW returns the EW stationkeeping decision for a GEO satellite.
// now is the evaluation epoch; drift is computed by the propagator.
func (pl *Planner) EvaluateEW(el model.Elements, now model.Epoch) (*EWDecision, error) {
	if !orbmath.IsGEO(el.A, el.I) {
		return &EWDecision{Required: false}, nil
	}
	drift, err := pl.prop.DriftRateDegPerDay(el)
	if err != nil {
		return nil, err
	}
	dec := &EWDecision{Drift: drift}
	if math.Abs(drift) <= orbmath.DriftDeadbandDegPerDay {
		return dec, nil
	}
	// crossing is "now" (drift already past deadband). The lead-time clock starts now.
	dec.CrossingAt = now
	dec.LatestExecAt = now + model.Epoch(orbmath.EWLeadTimeDays*86400)
	// target correction: d_a = -drift * K  (km; K locked km·day/deg)
	da := -drift * orbmath.EWGainKmDayPerDeg
	dec.TargetSMAKm = el.A + da
	// tangential delta-v = -0.5 * n * d_a  (m/s). n in rad/s, d_a in km.
	n := orbmath.MeanMotion(el.A)
	dec.DeltaVMps = -0.5 * n * da * 1000.0
	dec.Required = true
	return dec, nil
}

// ValidateExecTime enforces the locked lead-time constraint: the executed-at
// epoch must not be later than LatestExecAt. Returns ErrManeuverTooLate on
// violation. nowRef is unused but kept for symmetry.
func ValidateExecTime(dec *EWDecision, execAt model.Epoch) error {
	if dec == nil || !dec.Required {
		return nil
	}
	if execAt > dec.LatestExecAt {
		return model.ErrManeuverTooLate
	}
	return nil
}

// Overlap reports whether two maneuver execution windows [pa,pe] and
// [sa,se] overlap in time. Zero-length windows (un-executed, execAt=0) are
// treated as planned windows using PlannedAt as a 1-second window.
type Window struct{ Start, End model.Epoch }

func ManeuverWindow(m model.Maneuver) Window {
	if m.ExecutedAt != 0 {
		return Window{Start: m.ExecutedAt, End: m.ExecutedAt + 1}
	}
	return Window{Start: m.PlannedAt, End: m.PlannedAt + 1}
}

func Overlap(a, b Window) bool {
	return a.Start <= b.End && b.Start <= a.End
}

// PlanEW builds a Maneuver record for an EW burn from a decision and an
// execution epoch, validating lead-time. Returns the maneuver (status=planned)
// or an error.
func (pl *Planner) PlanEW(satID string, el model.Elements, dec *EWDecision, execAt model.Epoch, now model.Epoch) (model.Maneuver, error) {
	if dec == nil || !dec.Required {
		return model.Maneuver{}, model.ErrStateConflict
	}
	if err := ValidateExecTime(dec, execAt); err != nil {
		return model.Maneuver{}, err
	}
	target := el
	target.A = dec.TargetSMAKm
	target.Epoch = execAt
	return model.Maneuver{
		SatelliteID: satID,
		Type:         model.ManeuverEWStationKeep,
		PlannedAt:    now,
		ExecutedAt:   execAt,
		DeltaVMps:    dec.DeltaVMps,
		Target:       target,
		Status:       model.ManeuverPlanned,
		CreatedAt:    now,
	}, nil
}

// PlanPerigeeRaise decides whether a LEO satellite needs a perigee raise.
// If perigee altitude is below threshold it returns a planned maneuver
// raising perigee to threshold; otherwise returns (zero, false, nil).
func (pl *Planner) PlanPerigeeRaise(satID string, el model.Elements, B float64, now model.Epoch) (model.Maneuver, bool, error) {
	hp := orbmath.PerigeeAltitudeKm(el.A, el.E)
	if hp >= orbmath.PerigeeRaiseThresholdKm {
		return model.Maneuver{}, false, nil
	}
	// raise perigee: new a so that a(1-e) - Req = threshold => a' = (threshold+Req)/(1-e)
	newA := (orbmath.PerigeeRaiseThresholdKm + orbmath.Req) / (1 - el.E)
	da := newA - el.A
	// approx Hohmann first-impulse delta-v = 0.5 * v_orbit * (da/a)
	v := math.Sqrt(orbmath.Mu * (2.0/(el.A) - 1.0/el.A))
	dv := 0.5 * v * (da / el.A) * 1000.0 // m/s
	target := el
	target.A = newA
	target.Epoch = now
	return model.Maneuver{
		SatelliteID: satID,
		Type:         model.ManeuverPerigeeRaise,
		PlannedAt:    now,
		ExecutedAt:   now,
		DeltaVMps:    dv,
		Target:       target,
		Status:       model.ManeuverPlanned,
		CreatedAt:    now,
	}, true, nil
}

// PlanCollisionAvoidance builds an avoidance maneuver. It accepts the computed
// avoidance delta-v and the post-avoidance minimum distance; if the latter
// does not clear the safe threshold it returns ErrAvoidanceInsufficient.
func PlanCollisionAvoidance(satID, alertID string, dvMps float64, postMinDistKm float64, execAt model.Epoch, targetEl model.Elements, now model.Epoch) (model.Maneuver, error) {
	if postMinDistKm < orbmath.CollisionSafeKm {
		return model.Maneuver{}, model.ErrAvoidanceInsufficient
	}
	target := targetEl
	target.Epoch = execAt
	return model.Maneuver{
		SatelliteID: satID,
		Type:         model.ManeuverCollisionAvoid,
		PlannedAt:    now,
		ExecutedAt:   execAt,
		DeltaVMps:    dvMps,
		Target:       target,
		Status:       model.ManeuverPlanned,
		ReasonAlertID: alertID,
		CreatedAt:    now,
	}, nil
}

// CheckConflictAgainstExisting returns ErrManeuverConflict if the planned
// maneuver's execution window overlaps any of the existing same-satellite
// maneuvers that are still planned or executing.
func CheckConflictAgainstExisting(planned model.Maneuver, existing []model.Maneuver) error {
	pw := ManeuverWindow(planned)
	for _, m := range existing {
		if m.SatelliteID != planned.SatelliteID {
			continue
		}
		if m.Status != model.ManeuverPlanned && m.Status != model.ManeuverExecuting {
			continue
		}
		if Overlap(pw, ManeuverWindow(m)) {
			return model.ErrManeuverConflict
		}
	}
	return nil
}
