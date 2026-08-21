// Package orbmath implements the orbital-mechanics math primitives shared by
// the propagator, groundtrack, maneuver and colliance packages. It is pure:
// no I/O, no clocks, no allocations beyond the math itself. All angle inputs
// are degrees at the API boundary and radians internally. All distances are
// kilometers; velocities km/s; time is model.Epoch (seconds since J2000).
//
// The constants and formulae here are the locked interpretation: selfchecks
// and tests derive their expected values from these exact definitions.
package orbmath

import (
	"math"

	"orbitops/internal/model"
)

// Locked physical constants (SI-derived, km).
const (
	Mu       = 398600.4418 // km^3/s^2, Earth gravitational parameter
	Req      = 6378.137    // km, equatorial radius
	Flatten  = 1.0 / 298.257223563
	Rpol     = Req * (1 - Flatten)
	J2       = 1.08262668e-3
	OmegaE   = 7.2921151467e-5 // rad/s, Earth sidereal spin rate
	AGeo     = 42164.0         // km, geostationary semi-major axis (sidereal day)
	KmPerM   = 0.001
	Deg2Rad  = math.Pi / 180.0
	Rad2Deg  = 180.0 / math.Pi
	TwoPi    = math.Pi * 2.0
)

// J2000 epoch day zero corresponds to JD 2451545.0 (2000-01-01T12:00:00 UTC).
const J2000JD = 2451545.0

// Kepler constants.
const (
	KeplerMaxIter    = 8
	KeplerTolerance  = 1e-9 // radians
	LightStepDefault = 60  // seconds, pass/visibility scan step
	RefineEpsilon    = 1.0 // seconds, refined crossing precision
)

// Maneuver policy constants (locked interpretation).
const (
	DriftDeadbandDegPerDay = 0.05 // |drift| beyond this triggers EW stationkeeping
	EWLeadTimeDays         = 0.5  // maneuver must execute within this many days of crossing
	EWGainKmDayPerDeg      = 0.5  // semi-major axis correction gain (km·day/deg)
	CollisionAlertKm       = 10.0
	CollisionSafeKm        = 20.0
	HardBodyRadiusKm       = 0.05 // 50 m combined
	UncertaintyKm          = 1.0  // 1-sigma combined position
	PerigeeRaiseThresholdKm = 200.0
	DefaultBallisticCoef   = 0.01 // m^2/kg, Cd·A/m
	AtmosRho0              = 1.225 // kg/m^3 at sea level
	AtmosScaleHeightKm    = 7.0
)

// D2R converts degrees to radians.
func D2R(d float64) float64 { return d * Deg2Rad }

// R2D converts radians to degrees.
func R2D(r float64) float64 { return r * Rad2Deg }

// NormalizeDeg wraps an angle in degrees to [-180,180).
func NormalizeDeg(d float64) float64 {
	d = math.Mod(d, 360.0)
	if d >= 180.0 {
		d -= 360.0
	} else if d < -180.0 {
		d += 360.0
	}
	return d
}

// NormalizeRad wraps an angle in radians to [0,2π).
func NormalizeRad(r float64) float64 {
	r = math.Mod(r, TwoPi)
	if r < 0 {
		r += TwoPi
	}
	return r
}

// MeanMotion returns n = sqrt(mu/a^3) in rad/s for semi-major axis a (km).
func MeanMotion(a float64) float64 {
	return math.Sqrt(Mu / (a * a * a))
}

// SolveKepler solves Kepler's equation M = E - e*sin(E) for E given mean
// anomaly M and eccentricity e (radians, dimensionless). Newton iteration
// starts at E0=M and runs up to KeplerMaxIter; convergence requires
// |M-(E-e sin E)| < KeplerTolerance. Returns ErrKeplerDiverge if it does not
// converge. 0<=e<1 is required (validated upstream).
func SolveKepler(M, e float64) (float64, error) {
	if e < 0 || e > 1 {
		return 0, model.ErrInvalidElements
	}
	E := M
	for i := 0; i < KeplerMaxIter; i++ {
		f := E - e*math.Sin(E) - M
		if math.Abs(f) < KeplerTolerance {
			return E, nil
		}
		fp := 1 - e*math.Cos(E)
		if fp == 0 {
			break
		}
		E = E - f/fp
	}
	// final check
	f := E - e*math.Sin(E) - M
	if math.Abs(f) < KeplerTolerance {
		return E, nil
	}
	return 0, model.ErrKeplerDiverge
}

// TrueAnomaly returns the true anomaly ν (radians) from eccentric anomaly E
// and eccentricity e using the half-angle tangent form.
func TrueAnomaly(E, e float64) float64 {
	return 2.0 * math.Atan2(math.Sqrt(1+e)*math.Sin(E/2), math.Sqrt(1-e)*math.Cos(E/2))
}

// PerifocalPosition returns the position in the orbital plane (perifocal
// frame) from elements advanced to the target mean anomaly: r_pf = (a(cos E -
// e), a*sqrt(1-e^2) sin E). Also returns the orbital radius r = a(1 - e cos E).
func PerifocalPosition(a, e, E float64) (rx, ry, r float64) {
	rx = a * (math.Cos(E) - e)
	ry = a * math.Sqrt(1-e*e) * math.Sin(E)
	r = a * (1 - e*math.Cos(E))
	return
}

// Velocity from vis-viva v^2 = mu(2/r - 1/a), direction tangent in perifocal
// frame. Returns perifocal velocity components.
func PerifocalVelocity(a, e, E, r float64) (vx, vy, v float64) {
	v = math.Sqrt(Mu * (2.0/r - 1.0/a))
	// tangent direction: d(r_pf)/dE normalized
	dx := -a * math.Sin(E)
	dy := a * math.Sqrt(1-e*e) * math.Cos(E)
	norm := math.Sqrt(dx*dx + dy*dy)
	if norm == 0 {
		return 0, 0, 0
	}
	vx = v * dx / norm
	vy = v * dy / norm
	return
}

// Rotate3 applies a 3-1-3 (Ω, i, ω) rotation to a perifocal vector, yielding
// the J2000 ECI vector. R = Rz(Ω)·Rx(i)·Rz(ω). Angles in radians.
func Rotate3(v [3]float64, Omega, i, omega float64) [3]float64 {
	// Rz(omega)
	c, s := math.Cos(omega), math.Sin(omega)
	v = [3]float64{c*v[0] - s*v[1], s*v[0] + c*v[1], v[2]}
	// Rx(i)
	c, s = math.Cos(i), math.Sin(i)
	v = [3]float64{v[0], c*v[1] - s*v[2], s*v[1] + c*v[2]}
	// Rz(Omega)
	c, s = math.Cos(Omega), math.Sin(Omega)
	v = [3]float64{c*v[0] - s*v[1], s*v[0] + c*v[1], v[2]}
	return v
}

// GMST returns the Greenwich Apparent Sidereal Time in radians for the given
// epoch (seconds since J2000). θ = 280.4606° + 360.9856474°·d, d = epoch/86400.
// This is the mean sidereal time; we treat it as apparent for simplicity
// (locked interpretation).
func GMST(t model.Epoch) float64 {
	d := float64(t) / 86400.0
	gmstDeg := 280.4606 + 360.9856474*d
	return D2R(NormalizeDeg(gmstDeg))
}

// ECItoECEF rotates an ECI (J2000) vector into the Earth-fixed frame by
// rotating about Z by -θ_gmst.
func ECItoECEF(v [3]float64, gmst float64) [3]float64 {
	c, s := math.Cos(gmst), math.Sin(gmst)
	return [3]float64{c*v[0] + s*v[1], -s*v[0] + c*v[1], v[2]}
}

// ECEFtoECI is the inverse rotation (about Z by +θ_gmst).
func ECEFtoECI(v [3]float64, gmst float64) [3]float64 {
	c, s := math.Cos(gmst), math.Sin(gmst)
	return [3]float64{c*v[0] - s*v[1], s*v[0] + c*v[1], v[2]}
}

// ECEFtoLLA converts an Earth-fixed position to geodetic lat/lon/alt using the
// closed-form Bowring method on the WGS84-style ellipsoid (Req, Flatten).
// lon in [-180,180), lat in [-90,90], alt in km above ellipsoid.
func ECEFtoLLA(v [3]float64) model.LatLonAlt {
	x, y, z := v[0], v[1], v[2]
	p := math.Sqrt(x*x + y*y)
	if p == 0 {
		// pole
		lat := math.Pi / 2
		if z < 0 {
			lat = -lat
		}
		return model.LatLonAlt{Lat: R2D(lat), Lon: 0, Alt: math.Abs(z) - Rpol}
	}
	lon := math.Atan2(y, x)
	e2 := 2*Flatten - Flatten*Flatten
	ep2 := Flatten * (2 - Flatten) / ((1 - Flatten) * (1 - Flatten))
	theta := math.Atan2(z*Req, p*Rpol)
	lat := math.Atan2(z+ep2*Rpol*math.Pow(math.Sin(theta), 3),
		p-e2*Req*math.Pow(math.Cos(theta), 3))
	// altitude
	sinLat := math.Sin(lat)
	N := Req / math.Sqrt(1-e2*sinLat*sinLat)
	alt := p/math.Cos(lat) - N
	// numerical guard near poles
	if math.Abs(R2D(lat)) > 89.9 {
		alt = math.Abs(z) - Rpol*math.Sqrt(1-e2) // fallback
	}
	return model.LatLonAlt{Lat: R2D(lat), Lon: R2D(lon), Alt: alt}
}

// LLAtoECEF converts geodetic lat/lon/alt (degrees, km) to ECEF position.
func LLAtoECEF(latDeg, lonDeg, altKm float64) [3]float64 {
	lat := D2R(latDeg)
	lon := D2R(lonDeg)
	e2 := 2*Flatten - Flatten*Flatten
	sinLat := math.Sin(lat)
	N := Req / math.Sqrt(1-e2*sinLat*sinLat)
	x := (N + altKm) * math.Cos(lat) * math.Cos(lon)
	y := (N + altKm) * math.Cos(lat) * math.Sin(lon)
	z := (N*(1-e2) + altKm) * sinLat
	return [3]float64{x, y, z}
}

// ENUVectors returns the East/North/Up unit vectors (ECEF) at a geodetic
// location. Used to convert an ECEF relative vector into topocentric ENU.
func ENUVectors(latDeg, lonDeg float64) (e, n, u [3]float64) {
	lat := D2R(latDeg)
	lon := D2R(lonDeg)
	sl, cl := math.Sin(lat), math.Cos(lat)
	so, co := math.Sin(lon), math.Cos(lon)
	e = [3]float64{-so, co, 0}
	n = [3]float64{-sl * co, -sl * so, cl}
	u = [3]float64{cl * co, cl * so, sl}
	return
}

// RelativeENU returns the east/north/up components and range of the vector
// (sat - station) expressed in the station's ENU frame, plus elevation and
// azimuth (degrees). azimuth is clockwise from north [0,360).
func RelativeENU(satECEF, staECEF [3]float64, latDeg, lonDeg float64) (e, n, u, rangeKm, elevDeg, azDeg float64) {
	dx := [3]float64{satECEF[0] - staECEF[0], satECEF[1] - staECEF[1], satECEF[2] - staECEF[2]}
	east, north, up := ENUVectors(latDeg, lonDeg)
	e = dx[0]*east[0] + dx[1]*east[1] + dx[2]*east[2]
	n = dx[0]*north[0] + dx[1]*north[1] + dx[2]*north[2]
	u = dx[0]*up[0] + dx[1]*up[1] + dx[2]*up[2]
	rangeKm = math.Sqrt(e*e + n*n + u*u)
	if rangeKm == 0 {
		return
	}
	elevDeg = R2D(math.Asin(u / rangeKm))
	azDeg = R2D(math.Atan2(e, n))
	if azDeg < 0 {
		azDeg += 360
	}
	return
}

// PerigeeAltitudeKm returns the perigee altitude above the ellipsoid
// (approximate, using equatorial radius): h_p = a(1-e) - Req.
func PerigeeAltitudeKm(a, e float64) float64 {
	return a*(1-e) - Req
}

// J2SecularRates returns the secular drift rates (rad/s) of RAAN, argument of
// perigee and the mean-anomaly correction, for a given element set. These are
// first-order J2 averages; the satellite's a and e are held constant across
// the propagation window (small-eccentricity assumption).
func J2SecularRates(a, e, iDeg float64) (omegaDot, argpDot, mDot float64) {
	i := D2R(iDeg)
	n := MeanMotion(a)
	r := Req / a
	r2 := r * r
	e2 := e * e
	denom := (1 - e2) * (1 - e2)
	omegaDot = -1.5 * J2 * r2 * math.Cos(i) / denom * n
	argpDot = 1.5 * J2 * r2 * (2 - 2.5*math.Sin(i)*math.Sin(i)) / denom * n
	mDot = 1.5 * J2 * r2 * (1 - 3*math.Cos(i)*math.Cos(i)) / (denom * math.Sqrt(1-e2)) * n
	return
}

// DragPerigeeDecayKmPerRev estimates the perigee altitude loss per orbit
// revolution due to atmospheric drag, using an exponential atmosphere.
// B is the ballistic coefficient Cd·A/m (m^2/kg). a is semi-major axis (km).
// h_p is current perigee altitude (km). Returns km/rev (negative = decay).
func DragPerigeeDecayKmPerRev(B, a, hp float64) float64 {
	if B <= 0 {
		return 0
	}
	// density at perigee altitude (km above surface). Negative alt → dense.
	h := hp
	if h < 0 {
		h = 0
	}
	rho := AtmosRho0 * math.Exp(-h/AtmosScaleHeightKm)
	// simplified per-revolution decay: -2π·B·ρ·a (km), consistent with the
	// locked interpretation that decay scales with density and semi-major axis.
	return -TwoPi * B * rho * a
}

// SunDirectionECI returns a unit vector approximating the Earth-Sun direction
// in the J2000 ECI frame for the given epoch. Used by the cylindrical Earth
// shadow model. The approximation uses the Sun's mean ecliptic longitude; we
// assume the ecliptic lies in the ECI XY plane with obliquity folded into the
// declination term for a first-order result (locked interpretation).
func SunDirectionECI(t model.Epoch) [3]float64 {
	d := float64(t) / 86400.0
	lamSun := D2R(NormalizeDeg(280.460 + 0.9856474*d)) // mean ecliptic longitude
	obliquity := D2R(23.44)
	r := [3]float64{
		math.Cos(lamSun),
		math.Cos(obliquity) * math.Sin(lamSun),
		math.Sin(obliquity) * math.Sin(lamSun),
	}
	// normalize
	norm := math.Sqrt(r[0]*r[0] + r[1]*r[1] + r[2]*r[2])
	if norm == 0 {
		return r
	}
	return [3]float64{r[0] / norm, r[1] / norm, r[2] / norm}
}

// InEarthShadow reports whether the satellite ECI position is inside the
// cylindrical Earth shadow (umbra). The shadow cylinder is behind the Earth
// relative to the Sun; the satellite is in shadow if its perpendicular
// distance to the Earth-Sun line is less than Req AND it lies on the night
// side (r·sun_hat < 0).
func InEarthShadow(satECI [3]float64, sunDir [3]float64) bool {
	proj := satECI[0]*sunDir[0] + satECI[1]*sunDir[1] + satECI[2]*sunDir[2]
	if proj >= 0 {
		return false // on the sun side
	}
	perp := [3]float64{
		satECI[0] - proj*sunDir[0],
		satECI[1] - proj*sunDir[1],
		satECI[2] - proj*sunDir[2],
	}
	dist := math.Sqrt(perp[0]*perp[0] + perp[1]*perp[1] + perp[2]*perp[2])
	return dist < Req
}

// IsGEO classifies a satellite as near-geostationary (eligible for EW
// stationkeeping evaluation): a within +/-~130 km of AGeo and inclination
// below 5 degrees.
func IsGEO(a, iDeg float64) bool {
	return math.Abs(a-AGeo) < 130.0 && math.Abs(iDeg) < 5.0
}

// Clamp returns lo if v<lo, hi if v>hi, else v.
func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
