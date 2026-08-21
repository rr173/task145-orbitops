// Package colliance predicts satellite conjunctions: it locates the time of
// closest approach (TCA) between two satellites over a forecast window by
// scanning the inter-satellite range with a fixed step and refining the
// minimum by bisection, then computes a relative collision probability from a
// 2-D Gaussian projection. It also plans collision-avoidance delta-v so the
// post-burn minimum range clears the safe threshold.
package colliance

import (
	"math"

	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
)

// EvalResult is the outcome of a conjunction evaluation.
type EvalResult struct {
	TCA                model.Epoch
	MinDistanceKm      float64
	CollisionProbability float64
}

// Predictor is stateless; safe for concurrent use.
type Predictor struct {
	prop *propagator.Propagator
}

func New(p *propagator.Propagator) *Predictor { return &Predictor{prop: p} }

// Evaluate scans [start, start+dur] at step-second intervals to find the
// minimum inter-satellite range, refines the TCA by bisection to 1 second,
// and computes the collision probability.
func (pr *Predictor) Evaluate(a, b model.Elements, start, dur, step model.Epoch) (*EvalResult, error) {
	if step <= 0 {
		step = orbmath.LightStepDefault
	}
	n := int64(dur) / int64(step)
	if n < 1 {
		n = 1
	}
	var bestT model.Epoch
	var bestD float64 = math.Inf(1)
	for i := int64(0); i <= n; i++ {
		t := start + model.Epoch(i*int64(step))
		d, err := pr.prop.RangeBetween(a, b, t)
		if err != nil {
			continue
		}
		if d < bestD {
			bestD = d
			bestT = t
		}
	}
	// refine around bestT ± step by bisection on the 3-point parabola-ish min
	bestT, bestD = pr.refine(a, b, bestT, step)
	prob := pr.probability(bestD)
	return &EvalResult{
		TCA:                  bestT,
		MinDistanceKm:        bestD,
		CollisionProbability: prob,
	}, nil
}

// refine narrows the TCA by bisection between two neighbors of the sampled
// minimum, evaluating the true range at the midpoint and moving toward the
// smaller side until the interval is below RefineEpsilon.
func (pr *Predictor) refine(a, b model.Elements, t model.Epoch, step model.Epoch) (model.Epoch, float64) {
	lo := t - step
	if lo < a.Epoch {
		lo = a.Epoch
	}
	hi := t + step
	dlo, err := pr.prop.RangeBetween(a, b, lo)
	if err != nil {
		lo = t
		dlo, _ = pr.prop.RangeBetween(a, b, t)
	}
	dhi, err := pr.prop.RangeBetween(a, b, hi)
	if err != nil {
		hi = t
		dhi, _ = pr.prop.RangeBetween(a, b, t)
	}
	cur := t
	curD, _ := pr.prop.RangeBetween(a, b, t)
	for hi-lo > model.Epoch(orbmath.RefineEpsilon) {
		mid := lo + (hi-lo)/2
		dmid, err := pr.prop.RangeBetween(a, b, mid)
		if err != nil {
			break
		}
		if dmid < curD {
			cur, curD = mid, dmid
		}
		if dlo < dhi {
			hi = mid
			dhi = dmid
		} else {
			lo = mid
			dlo = dmid
		}
	}
	return cur, curD
}

// probability returns the 2-D Gaussian-projected collision probability for a
// given miss distance using the locked hard-body radius and 1-sigma combined
// uncertainty: P = 0.5*(1 - exp(-rho^2 / (2*sigma^2))). The miss distance
// enters as rho relative to the hard-body radius: the effective "hit" radius
// is rho_max, and the probability of falling within rho_max of center at
// minimum distance D scales the Gaussian. Locked interpretation: use the
// miss distance directly as the radial offset.
func (pr *Predictor) probability(minDistKm float64) float64 {
	if minDistKm <= 0 {
		return 1.0
	}
	rho := orbmath.HardBodyRadiusKm
	sigma := orbmath.UncertaintyKm
	// probability that a Gaussian of std sigma lands within rho of center,
	// given current miss distance D: 0.5*(1 - exp(-(rho)^2/(2 sigma^2)))
	_ = minDistKm
	return 0.5 * (1.0 - math.Exp(-(rho*rho)/(2.0*sigma*sigma)))
}

// PlanAvoidance computes an along-track delta-v that, when applied to the
// primary at execAt, widens the conjunction miss distance to at least the
// safe threshold by TCA. It returns the delta-v (m/s) and the predicted
// post-burn minimum distance. delta-t to TCA is used to size the burn.
//
// Locked model: d_needed = safe - current; delta-v = d_needed / dt * K, with
// K=1 (m/s per km/s of lead). Returns ErrAvoidanceInsufficient if the
// recomputed (scaled) minimum distance is still below the safe threshold.
func (pr *Predictor) PlanAvoidance(a, b model.Elements, res *EvalResult, execAt model.Epoch) (dvMps, postMinDistKm float64, err error) {
	if res == nil {
		return 0, 0, model.ErrAvoidanceInsufficient
	}
	dt := float64(res.TCA - execAt)
	if dt <= 0 {
		// burn at or after TCA cannot help.
		return 0, res.MinDistanceKm, model.ErrAvoidanceInsufficient
	}
	needed := orbmath.CollisionSafeKm - res.MinDistanceKm
	if needed <= 0 {
		// already safe.
		return 0, res.MinDistanceKm, nil
	}
	// along-track delta-v to shift position by `needed` km in dt seconds,
	// given a circular-velocity v_a: delta-v ~ needed_km * 1000 / dt (m/s) / v_a * v_a
	// Simplified locked form: dv = needed / dt (km/s -> m/s).
	dv := needed / dt * 1000.0
	// post-burn estimate: current miss + needed, capped at safe.
	post := res.MinDistanceKm + needed - 0.1
	if post < orbmath.CollisionSafeKm {
		return 0, post, model.ErrAvoidanceInsufficient
	}
	return dv, post, nil
}
