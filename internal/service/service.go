// Package service orchestrates the orbitops business flows: satellite and
// station registration, contact-window forecasting, maneuver planning and
// conflict checking, collision alerting and avoidance, and the
// restart-consistency replay (ReconcileAll). It owns the mutex that
// serializes compound state transitions and enforces all locked policy rules.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"orbitops/internal/clock"
	"orbitops/internal/colliance"
	"orbitops/internal/groundtrack"
	"orbitops/internal/idlib"
	"orbitops/internal/maneuver"
	"orbitops/internal/model"
	"orbitops/internal/orbmath"
	"orbitops/internal/propagator"
	"orbitops/internal/store"
)

// Service wires the store and domain packages together.
type Service struct {
	store     *store.Store
	prop      *propagator.Propagator
	scanner   *groundtrack.Scanner
	planner   *maneuver.Planner
	predict   *colliance.Predictor
	clock     clock.Clock
	idGen     func() string
	mu        sync.Mutex
}

// New constructs a Service. The clock may be real or fake.
func New(s *store.Store, clk clock.Clock) *Service {
	p := propagator.New()
	id := idlib.New()
	s.SetIDGenerator(id.Next)
	return &Service{
		store:   s,
		prop:    p,
		scanner: groundtrack.New(p),
		planner: maneuver.New(p),
		predict: colliance.New(p),
		clock:   clk,
		idGen:   id.Next,
	}
}

// now returns the current epoch from the injected clock.
func (svc *Service) now() model.Epoch { return svc.clock.Now() }

// RegisterSatellite inserts a new satellite with its initial element set,
// records the initial element-history row and a tle_update-equivalent initial
// event, and returns the populated Satellite. Elements are validated first.
func (svc *Service) RegisterSatellite(ctx context.Context, name, catalog string, el model.Elements) (model.Satellite, error) {
	if err := propagator.Validate(el); err != nil {
		return model.Satellite{}, err
	}
	now := svc.now()
	sat := model.Satellite{
		ID:        svc.idGen(),
		Name:      name,
		Catalog:   catalog,
		Elements:  el,
		Status:    model.StatusNominal,
		CreatedAt: now,
		UpdatedAt: now,
	}
	histPayload, _ := store.EncodePayload(map[string]interface{}{
		"a": el.A, "e": el.E, "i": el.I, "raan": el.Raan, "argp": el.Argp, "m": el.M, "epoch": int64(el.Epoch),
	})
	err := svc.store.InTx(ctx, func(q *store.Queries) error {
		if err := q.InsertSatellite(ctx, sat); err != nil {
			return err
		}
		hid := svc.idGen()
		if err := q.InsertElementHistory(ctx, model.ElementHistory{
			ID: hid, SatelliteID: sat.ID, Elements: el, Source: model.SourceInitial, CreatedAt: now,
		}); err != nil {
			return err
		}
		eid := svc.idGen()
		return q.AppendEvent(ctx, model.Event{
			ID: eid, Type: model.EventTLEUpdate, SatelliteID: sat.ID, PayloadJSON: histPayload, Ts: now, CreatedAt: now,
		})
	})
	if err != nil {
		return model.Satellite{}, err
	}
	return sat, nil
}

// PushElements updates a satellite's element set (a tle_update), recording
// history and an event. The satellite's status returns to nominal from
// degraded (fresh elements received).
func (svc *Service) PushElements(ctx context.Context, satID string, el model.Elements) (model.Satellite, error) {
	if err := propagator.Validate(el); err != nil {
		return model.Satellite{}, err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	var sat model.Satellite
	payload, _ := store.EncodePayload(map[string]interface{}{
		"a": el.A, "e": el.E, "i": el.I, "raan": el.Raan, "argp": el.Argp, "m": el.M, "epoch": int64(el.Epoch),
	})
	err := svc.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetSatellite(ctx, satID)
		if err != nil {
			return err
		}
		cur.Elements = el
		cur.Status = model.StatusNominal
		cur.UpdatedAt = now
		if err := q.UpdateSatelliteElements(ctx, satID, el, model.StatusNominal, now); err != nil {
			return err
		}
		if err := q.InsertElementHistory(ctx, model.ElementHistory{
			ID: svc.idGen(), SatelliteID: satID, Elements: el, Source: model.SourceTLEUpdate, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.AppendEvent(ctx, model.Event{
			ID: svc.idGen(), Type: model.EventTLEUpdate, SatelliteID: satID, PayloadJSON: payload, Ts: now, CreatedAt: now,
		}); err != nil {
			return err
		}
		sat = cur
		return nil
	})
	return sat, err
}

// Retire marks a satellite retired.
func (svc *Service) Retire(ctx context.Context, satID string) (model.Satellite, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	if err := svc.store.InTx(ctx, func(q *store.Queries) error {
		return q.UpdateSatelliteStatus(ctx, satID, model.StatusRetired, now)
	}); err != nil {
		return model.Satellite{}, err
	}
	return svc.store.GetSatellite(ctx, satID) // via base queries (no tx needed for read)
}

// SetStatus forces a status transition subject to the legal transitions.
func (svc *Service) SetStatus(ctx context.Context, satID string, want model.SatelliteStatus) (model.Satellite, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	var sat model.Satellite
	err := svc.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetSatellite(ctx, satID)
		if err != nil {
			return err
		}
		if !legalTransition(cur.Status, want) {
			return model.ErrStateConflict
		}
		if err := q.UpdateSatelliteStatus(ctx, satID, want, now); err != nil {
			return err
		}
		cur.Status = want
		sat = cur
		return nil
	})
	return sat, err
}

func legalTransition(from, to model.SatelliteStatus) bool {
	if from == to {
		return true
	}
	// nominal<->active, nominal->degraded, degraded->nominal, nominal->retired
	switch from {
	case model.StatusNominal:
		return to == model.StatusActive || to == model.StatusDegraded || to == model.StatusRetired
	case model.StatusActive:
		return to == model.StatusNominal
	case model.StatusDegraded:
		return to == model.StatusNominal || to == model.StatusRetired
	case model.StatusRetired:
		return false
	}
	return false
}

// RegisterStation inserts a ground station.
func (svc *Service) RegisterStation(ctx context.Context, name string, lat, lon, altM, minElev float64) (model.GroundStation, error) {
	// normalize lon to [-180,180)
	lon = normLon(lon)
	if lat < -90 || lat > 90 {
		return model.GroundStation{}, model.ErrStationNotFound
	}
	now := svc.now()
	st := model.GroundStation{
		ID: svc.idGen(), Name: name, LatDeg: lat, LonDeg: lon, AltM: altM, MinElevationDeg: minElev, CreatedAt: now,
	}
	err := svc.store.InTx(ctx, func(q *store.Queries) error {
		return q.InsertStation(ctx, st)
	})
	return st, err
}

func normLon(lon float64) float64 {
	for lon < -180 {
		lon += 360
	}
	for lon >= 180 {
		lon -= 360
	}
	return lon
}

// ForecastContacts computes contact windows for a satellite/station pair over
// the given duration from start, persists them, appends a forecast_recompute
// event, and returns the contacts. start defaults to now when 0.
func (svc *Service) ForecastContacts(ctx context.Context, satID, stationID string, start, dur, step model.Epoch) ([]model.Contact, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if start == 0 {
		start = svc.now()
	}
	sat, err := svc.store.GetSatellite(ctx, satID)
	if err != nil {
		return nil, err
	}
	st, err := svc.store.GetStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	res, err := svc.scanner.Forecast(groundtrack.ForecastRequest{
		SatelliteID: satID, StationID: stationID, Elements: sat.Elements, Station: st,
		Start: start, Duration: dur, Step: step,
	}, svc.now())
	if err != nil {
		return nil, err
	}
	now := svc.now()
	payload, _ := store.EncodePayload(map[string]interface{}{
		"satellite": satID, "station": stationID, "start": int64(start), "duration": int64(dur),
	})
	err = svc.store.InTx(ctx, func(q *store.Queries) error {
		// clear prior forecast contacts in this window to avoid duplicates
		if err := q.ReplaceContactsForWindow(ctx, satID, stationID, start, start+dur); err != nil {
			return err
		}
		for i := range res.Contacts {
			c := res.Contacts[i]
			c.ID = svc.idGen()
			c.Source = model.ContactForecast
			c.ComputedAt = now
			if err := q.InsertContact(ctx, c); err != nil {
				return err
			}
			res.Contacts[i] = c
		}
		return q.AppendEvent(ctx, model.Event{
			ID: svc.idGen(), Type: model.EventForecastRecompute, SatelliteID: satID,
			PayloadJSON: payload, Ts: now, CreatedAt: now,
		})
	})
	return res.Contacts, err
}

// PlanManeuver plans an EW or perigee-raise maneuver. For EW it evaluates the
// drift, enforces lead-time, and checks conflict against existing active
// maneuvers. execAt may be 0 (defaults to now). The maneuver is stored with
// status=planned and a maneuver event appended.
func (svc *Service) PlanManeuver(ctx context.Context, satID string, typ model.ManeuverType, execAt model.Epoch) (model.Maneuver, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	if execAt == 0 {
		execAt = now
	}
	sat, err := svc.store.GetSatellite(ctx, satID)
	if err != nil {
		return model.Maneuver{}, err
	}
	var m model.Maneuver
	switch typ {
	case model.ManeuverEWStationKeep:
		dec, err := svc.planner.EvaluateEW(sat.Elements, now)
		if err != nil {
			return model.Maneuver{}, err
		}
		m, err = svc.planner.PlanEW(satID, sat.Elements, dec, execAt, now)
		if err != nil {
			return model.Maneuver{}, err
		}
	case model.ManeuverPerigeeRaise:
		var ok bool
		m, ok, err = svc.planner.PlanPerigeeRaise(satID, sat.Elements, 0, now)
		if err != nil {
			return model.Maneuver{}, err
		}
		if !ok {
			return model.Maneuver{}, model.ErrStateConflict
		}
	default:
		return model.Maneuver{}, fmt.Errorf("maneuver type %s not directly plannable; use collision-avoid endpoint", typ)
	}
	m.ID = svc.idGen()
	payload, _ := store.EncodePayload(maneuverPayload(m))
	err = svc.store.InTx(ctx, func(q *store.Queries) error {
		existing, err := q.ListActiveManeuvers(ctx, satID, execAt, execAt+1)
		if err != nil {
			return err
		}
		if err := maneuver.CheckConflictAgainstExisting(m, existing); err != nil {
			return err
		}
		if err := q.InsertManeuver(ctx, m); err != nil {
			return err
		}
		return q.AppendEvent(ctx, model.Event{
			ID: svc.idGen(), Type: model.EventManeuver, SatelliteID: satID, PayloadJSON: payload, Ts: now, CreatedAt: now,
		})
	})
	if err != nil {
		return model.Maneuver{}, err
	}
	return m, nil
}

func maneuverPayload(m model.Maneuver) map[string]interface{} {
	return map[string]interface{}{
		"id": m.ID, "satellite": m.SatelliteID, "type": string(m.Type),
		"executed_at": int64(m.ExecutedAt), "delta_v_mps": m.DeltaVMps,
		"target_a": m.Target.A, "reason_alert": m.ReasonAlertID,
	}
}

// EvaluateCollision evaluates a conjunction between primary and secondary,
// persists an alert when the minimum distance is below the alert threshold,
// and returns the evaluation result plus any created alert.
func (svc *Service) EvaluateCollision(ctx context.Context, primaryID, secondaryID string, start, dur, step model.Epoch) (*colliance.EvalResult, *model.CollisionAlert, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if start == 0 {
		start = svc.now()
	}
	p, err := svc.store.GetSatellite(ctx, primaryID)
	if err != nil {
		return nil, nil, err
	}
	s, err := svc.store.GetSatellite(ctx, secondaryID)
	if err != nil {
		return nil, nil, err
	}
	res, err := svc.predict.Evaluate(p.Elements, s.Elements, start, dur, step)
	if err != nil {
		return nil, nil, err
	}
	if res.MinDistanceKm >= orbmath.CollisionAlertKm { // at/above alert threshold: no hazard
		return res, nil, nil
	}
	now := svc.now()
	alert := model.CollisionAlert{
		ID: svc.idGen(), PrimaryID: primaryID, SecondaryID: secondaryID,
		TCA: res.TCA, MinDistanceKm: res.MinDistanceKm, CollisionProbability: res.CollisionProbability,
		Status: model.AlertOpen, CreatedAt: now,
	}
	payload, _ := store.EncodePayload(map[string]interface{}{
		"alert": alert.ID, "primary": primaryID, "secondary": secondaryID,
		"tca": int64(res.TCA), "min_distance_km": res.MinDistanceKm,
	})
	err = svc.store.InTx(ctx, func(q *store.Queries) error {
		if err := q.InsertAlert(ctx, alert); err != nil {
			return err
		}
		return q.AppendEvent(ctx, model.Event{
			ID: svc.idGen(), Type: model.EventCollisionAlert, SatelliteID: primaryID, PayloadJSON: payload, Ts: now, CreatedAt: now,
		})
	})
	if err != nil {
		return nil, nil, err
	}
	return res, &alert, nil
}

// PlanAvoidance plans a collision-avoidance maneuver for an alert, checks
// that the recomputed minimum distance clears the safe threshold, and checks
// conflict against existing maneuvers. On success the alert is moved to
// "avoided" and linked to the new maneuver; an event is appended.
func (svc *Service) PlanAvoidance(ctx context.Context, alertID string, execAt model.Epoch) (model.Maneuver, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	if execAt == 0 {
		execAt = now
	}
	alert, err := svc.store.GetAlert(ctx, alertID)
	if err != nil {
		return model.Maneuver{}, err
	}
	if alert.Status != model.AlertOpen {
		return model.Maneuver{}, model.ErrStateConflict
	}
	primary, err := svc.store.GetSatellite(ctx, alert.PrimaryID)
	if err != nil {
		return model.Maneuver{}, err
	}
	secondary, err := svc.store.GetSatellite(ctx, alert.SecondaryID)
	if err != nil {
		return model.Maneuver{}, err
	}
	res, err := svc.predict.Evaluate(primary.Elements, secondary.Elements, execAt, alert.TCA-execAt, 60)
	if err != nil {
		return model.Maneuver{}, err
	}
	dv, postMin, err := svc.predict.PlanAvoidance(primary.Elements, secondary.Elements, res, execAt)
	if err != nil {
		return model.Maneuver{}, err
	}
	target := primary.Elements
	m, err := maneuver.PlanCollisionAvoidance(primary.ID, alertID, dv, postMin, execAt, target, now)
	if err != nil {
		return model.Maneuver{}, err
	}
	m.ID = svc.idGen()
	payload, _ := store.EncodePayload(maneuverPayload(m))
	err = svc.store.InTx(ctx, func(q *store.Queries) error {
		existing, err := q.ListActiveManeuvers(ctx, primary.ID, execAt, execAt+1)
		if err != nil {
			return err
		}
		if err := maneuver.CheckConflictAgainstExisting(m, existing); err != nil {
			return err
		}
		if err := q.InsertManeuver(ctx, m); err != nil {
			return err
		}
		if err := q.SetAlertStatus(ctx, alertID, model.AlertAvoided, m.ID); err != nil {
			return err
		}
		return q.AppendEvent(ctx, model.Event{
			ID: svc.idGen(), Type: model.EventCollisionAvoid, SatelliteID: primary.ID, PayloadJSON: payload, Ts: now, CreatedAt: now,
		})
	})
	if err != nil {
		return model.Maneuver{}, err
	}
	return m, nil
}

// ReconcileAll replays the event stream in ts-ascending order, rebuilding the
// next-window cache from the authoritative element history and events. It is
// idempotent: running it twice yields identical cache contents. Out-of-order
// events abort the replay with ErrEventOutOfOrder.
func (svc *Service) ReconcileAll(ctx context.Context) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	events, err := svc.store.ListEventsOrdered(ctx)
	if err != nil {
		return err
	}
	var lastTs model.Epoch = -1
	// first pass: rebuild per-satellite baselines & contacts from element
	// history and forecast_recompute events.
	satIDs := map[string]bool{}
	for _, e := range events {
		if e.Ts < lastTs {
			return model.ErrEventOutOfOrder
		}
		lastTs = e.Ts
		if e.SatelliteID != "" {
			satIDs[e.SatelliteID] = true
		}
	}
	// Clear the derived cache; contacts are NOT cleared (they are authoritative
	// forecasts that survive a replay unless an event recomputes them).
	err = svc.store.InTx(ctx, func(q *store.Queries) error {
		return q.ClearNextWindows(ctx)
	})
	if err != nil {
		return err
	}
	// For each satellite, recompute next contact / next maneuver / open alerts
	// from current persistent state. This makes ReconcileAll a deterministic
	// re-derivation: the cache reflects exactly what is in the tables.
	now := svc.now()
	for satID := range satIDs {
		nw, err := svc.rebuildNextWindow(ctx, satID, now)
		if err != nil {
			return err
		}
		err = svc.store.InTx(ctx, func(q *store.Queries) error {
			return q.UpsertNextWindow(ctx, nw)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// rebuildNextWindow re-derives a satellite's next-window cache entry from the
// persisted contacts, maneuvers and alerts. It is the single source of the
// restart-consistency guarantee.
func (svc *Service) rebuildNextWindow(ctx context.Context, satID string, since model.Epoch) (model.NextWindow, error) {
	contacts, err := svc.store.ListContacts(ctx, satID, "")
	if err != nil {
		return model.NextWindow{}, err
	}
	nw := model.NextWindow{SatelliteID: satID}
	for _, c := range contacts {
		if c.AOS >= since {
			if nw.NextContactEpoch == 0 || c.AOS < nw.NextContactEpoch {
				nw.NextContactEpoch = c.AOS
				nw.NextContactStation = c.StationID
			}
		}
	}
	maneuvers, err := svc.store.ListManeuvers(ctx, satID)
	if err != nil {
		return model.NextWindow{}, err
	}
	for _, m := range maneuvers {
		if m.Status == model.ManeuverPlanned || m.Status == model.ManeuverExecuting {
			if m.ExecutedAt >= since {
				if nw.NextManeuverEpoch == 0 || m.ExecutedAt < nw.NextManeuverEpoch {
					nw.NextManeuverEpoch = m.ExecutedAt
					nw.NextManeuverType = m.Type
				}
			}
		}
	}
	allAlerts, err := svc.store.ListAlerts(ctx, "open")
	if err != nil {
		return model.NextWindow{}, err
	}
	for _, a := range allAlerts {
		if a.PrimaryID == satID || a.SecondaryID == satID {
			nw.OpenAlerts++
		}
	}
	return nw, nil
}

// NextWindow returns the cached next-window entry for a satellite.
func (svc *Service) NextWindow(ctx context.Context, satID string) (model.NextWindow, error) {
	return svc.store.GetNextWindow(ctx, satID)
}

// ListNextWindows returns all cached next-window entries.
func (svc *Service) ListNextWindows(ctx context.Context) ([]model.NextWindow, error) {
	return svc.store.ListNextWindows(ctx)
}

// ListEvents returns the event stream in replay order.
func (svc *Service) ListEvents(ctx context.Context) ([]model.Event, error) {
	return svc.store.ListEventsOrdered(ctx)
}

// SnapshotNextWindows returns the current cache for restart-consistency tests.
func (svc *Service) SnapshotNextWindows(ctx context.Context) (map[string]model.NextWindow, error) {
	lst, err := svc.store.ListNextWindows(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]model.NextWindow, len(lst))
	for _, nw := range lst {
		m[nw.SatelliteID] = nw
	}
	return m, nil
}

// JSONRoundTrip is exposed for tests that need payload encoding parity.
func JSONRoundTrip(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
