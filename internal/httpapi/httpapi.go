// Package httpapi wires the HTTP handlers for orbitops. It exposes a single
// Router() that returns an http.Handler usable by both the live service and
// the self-check smoke harness. Every handler maps a domain error to a
// suitable HTTP status; success responses are JSON.
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"orbitops/internal/clock"
	"orbitops/internal/colliance"
	"orbitops/internal/model"
	"orbitops/internal/propagator"
	"orbitops/internal/service"
	"orbitops/internal/store"
	"orbitops/internal/webfs"
)

// Server holds shared dependencies for the handlers.
type Server struct {
	svc     *service.Service
	store   *store.Store
	prop    *propagator.Propagator
	predict *colliance.Predictor
	clock   clock.Clock
}

// New constructs a Server bound to a Service and store.
func New(svc *service.Service, st *store.Store, clk clock.Clock) *Server {
	p := propagator.New()
	return &Server{
		svc:     svc,
		store:   st,
		prop:    p,
		predict: colliance.New(p),
		clock:   clk,
	}
}

// Router builds the full handler tree and returns it.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Front-end (embedded).
	mux.Handle("/", http.FileServer(webfs.StaticFS()))

	// Health.
	mux.HandleFunc("/api/health", s.health)

	// Satellites.
	mux.HandleFunc("/api/satellites", s.satellites)
	mux.HandleFunc("/api/satellites/", s.satelliteByID) // :id and sub-paths

	// Ground stations.
	mux.HandleFunc("/api/groundstations", s.stations)
	mux.HandleFunc("/api/groundstations/", s.stationByID)

	// Propagation & ground track.
	mux.HandleFunc("/api/propagate", s.propagate)
	mux.HandleFunc("/api/groundtrack", s.groundtrack)

	// Contacts.
	mux.HandleFunc("/api/contacts/forecast", s.forecastContacts)
	mux.HandleFunc("/api/contacts", s.listContacts)
	mux.HandleFunc("/api/contacts/", s.contactByID)

	// Maneuvers.
	mux.HandleFunc("/api/maneuvers", s.maneuvers)
	mux.HandleFunc("/api/maneuvers/", s.maneuverByID)

	// Collision.
	mux.HandleFunc("/api/colliance/evaluate", s.evalCollision)
	mux.HandleFunc("/api/colliance/alerts", s.listAlerts)
	mux.HandleFunc("/api/colliance/alerts/", s.alertByID)

	// Reconcile & events.
	mux.HandleFunc("/api/recompute", s.recompute)
	mux.HandleFunc("/api/events", s.events)

	// Admin / report.
	mux.HandleFunc("/api/admin/reset", s.adminReset)
	mux.HandleFunc("/api/report/constellation", s.constellation)
	mux.HandleFunc("/api/nextwindows", s.nextWindows)

	return s.withLogging(mux)
}

// withLogging wraps the handler with request logging.
func (s *Server) withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
}

// --------------------------------------------------------------------------- //
// helpers
// --------------------------------------------------------------------------- //

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var de *model.Error
	if errors.As(err, &de) {
		status := statusFor(de.Code)
		writeJSON(w, status, map[string]string{"error": de.Code, "message": de.Msg})
		return
	}
	log.Printf("internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "message": err.Error()})
}

func statusFor(code string) int {
	switch code {
	case "satellite_not_found", "station_not_found", "maneuver_not_found",
		"alert_not_found", "contact_not_found":
		return http.StatusNotFound
	case "invalid_elements", "kepler_diverge":
		return http.StatusUnprocessableEntity
	case "state_conflict", "maneuver_conflict", "maneuver_too_late",
		"avoidance_insufficient", "event_out_of_order":
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func parseJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// trimPrefix removes the leading slash from a path segment.
func trimPrefix(s string) string { return strings.TrimPrefix(s, "/") }

// firstSeg returns the first path segment after the handler prefix.
func firstSeg(rest string) (seg, remainder string) {
	rest = strings.TrimPrefix(rest, "/")
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest, ""
	}
	return rest[:idx], rest[idx+1:]
}
