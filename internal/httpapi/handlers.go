package httpapi

import (
	"net/http"
	"strings"

	"orbitops/internal/model"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --------------------------------------------------------------------------- //
// Satellites
// --------------------------------------------------------------------------- //

type registerSatReq struct {
	Name    string  `json:"name"`
	Catalog string  `json:"catalog"`
	A       float64 `json:"a"`
	E       float64 `json:"e"`
	I       float64 `json:"i"`
	Raan    float64 `json:"raan"`
	Argp    float64 `json:"argp"`
	M       float64 `json:"m"`
	Epoch   int64   `json:"epoch"`
}

func (s *Server) satellites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		lst, err := s.store.ListSatellites(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, lst)
	case http.MethodPost:
		var req registerSatReq
		if err := parseJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json", "message": err.Error()})
			return
		}
		el := model.Elements{A: req.A, E: req.E, I: req.I, Raan: req.Raan, Argp: req.Argp, M: req.M, Epoch: model.Epoch(req.Epoch)}
		sat, err := s.svc.RegisterSatellite(r.Context(), req.Name, req.Catalog, el)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, sat)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) satelliteByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/satellites/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	// sub-paths: /elements, /elements/history, /status
	rest := strings.TrimPrefix(r.URL.Path, "/api/satellites/"+id)
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case rest == "" || rest == "/":
		switch r.Method {
		case http.MethodGet:
			sat, err := s.store.GetSatellite(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, sat)
		case http.MethodDelete:
			sat, err := s.svc.Retire(r.Context(), id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, sat)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		}
	case rest == "status":
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := parseJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
			return
		}
		sat, err := s.svc.SetStatus(r.Context(), id, model.SatelliteStatus(body.Status))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sat)
	case rest == "elements/history":
		lst, err := s.store.ListElementHistory(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, lst)
	case rest == "elements":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var req registerSatReq
		if err := parseJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
			return
		}
		el := model.Elements{A: req.A, E: req.E, I: req.I, Raan: req.Raan, Argp: req.Argp, M: req.M, Epoch: model.Epoch(req.Epoch)}
		sat, err := s.svc.PushElements(r.Context(), id, el)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sat)
	default:
		http.NotFound(w, r)
	}
}

// --------------------------------------------------------------------------- //
// Ground stations
// --------------------------------------------------------------------------- //

type registerStationReq struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	AltM    float64 `json:"alt_m"`
	MinElev float64 `json:"min_elevation"`
}

func (s *Server) stations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		lst, err := s.store.ListStations(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, lst)
	case http.MethodPost:
		var req registerStationReq
		if err := parseJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
			return
		}
		st, err := s.svc.RegisterStation(r.Context(), req.Name, req.Lat, req.Lon, req.AltM, req.MinElev)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, st)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) stationByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/groundstations/")
	st, err := s.store.GetStation(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// --------------------------------------------------------------------------- //
// Propagate & ground track
// --------------------------------------------------------------------------- //

type propagateReq struct {
	SatelliteID string `json:"satellite_id"`
	AtEpoch     int64  `json:"at_epoch"`
}

type propagateResp struct {
	Epoch int64            `json:"epoch"`
	ECI   model.StateVector `json:"eci"`
	LLA   model.LatLonAlt  `json:"lla"`
}

func (s *Server) propagate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var req propagateReq
	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	sat, err := s.store.GetSatellite(r.Context(), req.SatelliteID)
	if err != nil {
		writeError(w, err)
		return
	}
	lla, sv, err := s.prop.SubPoint(sat.Elements, model.Epoch(req.AtEpoch))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, propagateResp{Epoch: req.AtEpoch, ECI: sv, LLA: lla})
}

type groundTrackReq struct {
	SatelliteID string `json:"satellite_id"`
	Start       int64  `json:"start"`
	Steps       int    `json:"steps"`
	StepSec     int64  `json:"step_sec"`
}

func (s *Server) groundtrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var req groundTrackReq
	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	if req.Steps <= 0 || req.Steps > 2000 {
		req.Steps = 100
	}
	if req.StepSec <= 0 {
		req.StepSec = 60
	}
	sat, err := s.store.GetSatellite(r.Context(), req.SatelliteID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]model.LatLonAlt, 0, req.Steps)
	t := model.Epoch(req.Start)
	for i := 0; i < req.Steps; i++ {
		lla, _, err := s.prop.SubPoint(sat.Elements, t)
		if err != nil {
			writeError(w, err)
			return
		}
		out = append(out, lla)
		t += model.Epoch(req.StepSec)
	}
	writeJSON(w, http.StatusOK, out)
}

// --------------------------------------------------------------------------- //
// Contacts
// --------------------------------------------------------------------------- //

type forecastReq struct {
	SatelliteID string `json:"satellite_id"`
	StationID   string `json:"station_id"`
	Start       int64  `json:"start"`
	Duration    int64  `json:"duration"`
	Step        int64  `json:"step"`
}

func (s *Server) forecastContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var req forecastReq
	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	contacts, err := s.svc.ForecastContacts(r.Context(), req.SatelliteID, req.StationID,
		model.Epoch(req.Start), model.Epoch(req.Duration), model.Epoch(req.Step))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contacts)
}

func (s *Server) listContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	q := r.URL.Query()
	lst, err := s.store.ListContacts(r.Context(), q.Get("satellite"), q.Get("station"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lst)
}

func (s *Server) contactByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/contacts/")
	c, err := s.store.GetContact(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// --------------------------------------------------------------------------- //
// Maneuvers
// --------------------------------------------------------------------------- //

type planManeuverReq struct {
	SatelliteID string `json:"satellite_id"`
	Type        string `json:"type"`
	ExecAt      int64  `json:"exec_at"`
}

func (s *Server) maneuvers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		lst, err := s.store.ListManeuvers(r.Context(), q.Get("satellite"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, lst)
	case http.MethodPost:
		var req planManeuverReq
		if err := parseJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
			return
		}
		m, err := s.svc.PlanManeuver(r.Context(), req.SatelliteID, model.ManeuverType(req.Type), model.Epoch(req.ExecAt))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, m)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) maneuverByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/maneuvers/")
	m, err := s.store.GetManeuver(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// --------------------------------------------------------------------------- //
// Collision
// --------------------------------------------------------------------------- //

type evalCollisionReq struct {
	PrimaryID   string `json:"primary_id"`
	SecondaryID string `json:"secondary_id"`
	Start       int64  `json:"start"`
	Duration    int64  `json:"duration"`
	Step        int64  `json:"step"`
}

func (s *Server) evalCollision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var req evalCollisionReq
	if err := parseJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
		return
	}
	res, alert, err := s.svc.EvaluateCollision(r.Context(), req.PrimaryID, req.SecondaryID,
		model.Epoch(req.Start), model.Epoch(req.Duration), model.Epoch(req.Step))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": res, "alert": alert})
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	status := r.URL.Query().Get("status")
	lst, err := s.store.ListAlerts(r.Context(), status)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lst)
}

func (s *Server) alertByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/colliance/alerts/")
	// rest may be "<id>" or "<id>/avoid"
	id, sub := firstSeg(rest)
	if sub == "avoid" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var body struct {
			ExecAt int64 `json:"exec_at"`
		}
		_ = parseJSON(r, &body)
		m, err := s.svc.PlanAvoidance(r.Context(), id, model.Epoch(body.ExecAt))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, m)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	a, err := s.store.GetAlert(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// --------------------------------------------------------------------------- //
// Reconcile, events, admin, report
// --------------------------------------------------------------------------- //

func (s *Server) recompute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if err := s.svc.ReconcileAll(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reconciled"})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	lst, err := s.store.ListEventsOrdered(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lst)
}

func (s *Server) adminReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if err := s.store.ResetAll(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

type constellationRow struct {
	Satellite        model.Satellite `json:"satellite"`
	NextContactEpoch int64           `json:"next_contact_epoch"`
	NextContactStation string        `json:"next_contact_station"`
	NextManeuverEpoch int64           `json:"next_maneuver_epoch"`
	NextManeuverType  string         `json:"next_maneuver_type"`
	OpenAlerts         int            `json:"open_alerts"`
}

func (s *Server) constellation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	sats, err := s.store.ListSatellites(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	rows := make([]constellationRow, 0, len(sats))
	for _, sat := range sats {
		nw, err := s.store.GetNextWindow(r.Context(), sat.ID)
		if err != nil && err == model.ErrSatelliteNotFound {
			nw = model.NextWindow{SatelliteID: sat.ID}
		} else if err != nil {
			writeError(w, err)
			return
		}
		rows = append(rows, constellationRow{
			Satellite:          sat,
			NextContactEpoch:   int64(nw.NextContactEpoch),
			NextContactStation: nw.NextContactStation,
			NextManeuverEpoch:  int64(nw.NextManeuverEpoch),
			NextManeuverType:   string(nw.NextManeuverType),
			OpenAlerts:         nw.OpenAlerts,
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) nextWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	lst, err := s.store.ListNextWindows(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lst)
}

// end of handlers
