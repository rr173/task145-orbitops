// Package model defines the domain types and state enums for the orbital
// mechanics engine. It has no external dependencies and is shared by every
// other package (store, service, httpapi, selfcheck).
package model

import "fmt"

// Epoch is the number of integer seconds elapsed since the J2000 reference
// epoch (2000-01-01T12:00:00 UTC). All orbit computations are driven purely
// by Epoch values to remain deterministic and replayable across restarts.
type Epoch int64

// Elements is a classical Keplerian element set (two-line element, minus the
// mean motion; we derive mean motion from the semi-major axis). Angles are in
// degrees in storage/API but converted to radians at the math boundary.
type Elements struct {
	A      float64 // semi-major axis, km
	E      float64 // eccentricity, 0<=E<1
	I      float64 // inclination, degrees
	Raan   float64 // right ascension of ascending node, degrees
	Argp   float64 // argument of perigee, degrees
	M      float64 // mean anomaly, degrees
	Epoch  Epoch  // epoch of element set (seconds since J2000)
}

// StateVector is an Earth-centered inertial (J2000) position/velocity in km and km/s.
type StateVector struct {
	X, Y, Z    float64 // km
	VX, VY, VZ float64 // km/s
}

// LatLonAlt is a geodetic position.
type LatLonAlt struct {
	Lat float64 // degrees
	Lon float64 // degrees, [-180,180)
	Alt float64 // km above ellipsoid
}

// SatelliteStatus enumerates the operational state of a satellite.
type SatelliteStatus string

const (
	StatusNominal  SatelliteStatus = "nominal"
	StatusActive   SatelliteStatus = "active"   // inside a maneuver execution window
	StatusDegraded SatelliteStatus = "degraded" // elements aged beyond validity window
	StatusRetired  SatelliteStatus = "retired"
)

// ElementSource records how an element set came to be the baseline.
type ElementSource string

const (
	SourceInitial  ElementSource = "initial"
	SourceManeuver ElementSource = "maneuver"
	SourceTLEUpdate ElementSource = "tle_update"
)

// ManeuverType enumerates supported maneuvers.
type ManeuverType string

const (
	ManeuverEWStationKeep  ManeuverType = "ew_stationkeep"
	ManeuverNSStationKeep  ManeuverType = "ns_stationkeep"
	ManeuverPerigeeRaise   ManeuverType = "perigee_raise"
	ManeuverCollisionAvoid ManeuverType = "collision_avoid"
)

// ManeuverStatus enumerates maneuver lifecycle states.
type ManeuverStatus string

const (
	ManeuverPlanned   ManeuverStatus = "planned"
	ManeuverExecuting ManeuverStatus = "executing"
	ManeuverDone      ManeuverStatus = "done"
	ManeuverCanceled  ManeuverStatus = "canceled"
)

// AlertStatus enumerates collision alert lifecycle states.
type AlertStatus string

const (
	AlertOpen     AlertStatus = "open"
	AlertAvoided  AlertStatus = "avoided"
	AlertDismissed AlertStatus = "dismissed"
)

// ContactSource records how a contact window was produced.
type ContactSource string

const (
	ContactForecast ContactSource = "forecast"
	ContactReplay   ContactSource = "replay"
	ContactAmbiguous ContactSource = "ambiguous" // segment could not be resolved
)

// EventType enumerates event-stream record kinds.
type EventType string

const (
	EventTLEUpdate        EventType = "tle_update"
	EventManeuver         EventType = "maneuver"
	EventForecastRecompute EventType = "forecast_recompute"
	EventCollisionAlert   EventType = "collision_alert"
	EventCollisionAvoid   EventType = "collision_avoid"
)

// Satellite is the top-level tracked object.
type Satellite struct {
	ID        string
	Name      string
	Catalog   string
	Elements  Elements
	Status    SatelliteStatus
	CreatedAt Epoch
	UpdatedAt Epoch
}

// GroundStation is a tracking station.
type GroundStation struct {
	ID             string
	Name           string
	LatDeg         float64
	LonDeg         float64
	AltM           float64
	MinElevationDeg float64
	CreatedAt      Epoch
}

// ElementHistory is an immutable element-set snapshot.
type ElementHistory struct {
	ID         string
	SatelliteID string
	Elements   Elements
	Source     ElementSource
	CreatedAt  Epoch
}

// Contact is a visibility window between a satellite and a ground station.
type Contact struct {
	ID              string
	SatelliteID     string
	StationID       string
	AOS             Epoch // acquisition of signal (rise)
	Los             Epoch // loss of signal (set)
	TCA             Epoch // time of closest approach
	MaxElevationDeg float64
	AOSAzDeg        float64
	LosAzDeg        float64
	Sunlit          bool
	Source          ContactSource
	ComputedAt      Epoch
}

// Maneuver is a planned or executed propulsive burn.
type Maneuver struct {
	ID            string
	SatelliteID   string
	Type          ManeuverType
	PlannedAt     Epoch
	ExecutedAt    Epoch
	DeltaVMps     float64
	Target        Elements
	Status        ManeuverStatus
	ReasonAlertID string // set for collision_avoid
	CreatedAt     Epoch
}

// CollisionAlert is a predicted conjunction hazard.
type CollisionAlert struct {
	ID                 string
	PrimaryID          string
	SecondaryID        string
	TCA                Epoch
	MinDistanceKm      float64
	CollisionProbability float64
	Status             AlertStatus
	AvoidManeuverID    string
	CreatedAt          Epoch
}

// Event is an event-stream record used for replay on restart.
type Event struct {
	ID          string
	Type        EventType
	SatelliteID string
	PayloadJSON string
	Ts          Epoch
	CreatedAt   Epoch
}

// NextWindow is the per-satellite cached "next" window summary rebuilt on
// reconcile; equality of these fields is the restart-consistency guarantee.
type NextWindow struct {
	SatelliteID        string
	NextContactEpoch   Epoch // 0 if none
	NextContactStation string
	NextManeuverEpoch  Epoch
	NextManeuverType   ManeuverType
	OpenAlerts         int
}

// Domain errors. Each maps to a distinct HTTP status in httpapi.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func NewErr(code, msg string) *Error { return &Error{Code: code, Msg: msg} }

var (
	ErrKeplerDiverge         = NewErr("kepler_diverge", "kepler equation did not converge within 8 iterations")
	ErrInvalidElements       = NewErr("invalid_elements", "elements invalid: require 0<=e<1 and a>0")
	ErrSatelliteNotFound     = NewErr("satellite_not_found", "satellite not found")
	ErrStationNotFound       = NewErr("station_not_found", "ground station not found")
	ErrStateConflict         = NewErr("state_conflict", "state transition not allowed")
	ErrManeuverTooLate       = NewErr("maneuver_too_late", "maneuver exceeds lead-time constraint")
	ErrManeuverConflict      = NewErr("maneuver_conflict", "maneuver window overlaps an existing maneuver")
	ErrAvoidanceInsufficient = NewErr("avoidance_insufficient", "avoidance delta-v does not clear safe threshold")
	ErrEventOutOfOrder       = NewErr("event_out_of_order", "event stream replayed out of order")
	ErrManeuverNotFound      = NewErr("maneuver_not_found", "maneuver not found")
	ErrAlertNotFound         = NewErr("alert_not_found", "collision alert not found")
	ErrContactNotFound      = NewErr("contact_not_found", "contact not found")
)
