// Package selfcheck runs the end-to-end smoke harness: register a satellite
// and station, forecast contacts, plan a maneuver, capture the next-window
// snapshot, restart-equivalent reconcile, and assert the snapshot is identical.
// It also requests the front-end page and a business API to prove the page is
// reachable and wired to real computation. On success it returns 0.
package selfcheck

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"

	"orbitops/internal/clock"
	"orbitops/internal/model"
	"orbitops/internal/service"
	"orbitops/internal/store"
)

// Run executes the smoke harness against an in-process server. It uses an
// httptest.Server so HTTP clients (the front-end flow uses net/http) exercise
// the real router. Returns 0 on success, non-zero on failure.
func Run(ctx context.Context, svc *service.Service, srv interface {
	Router() http.Handler
}, st *store.Store, clk clock.Clock) int {
	// reset to a clean slate
	if err := st.ResetAll(ctx); err != nil {
		fmt.Printf("reset: %v\n", err)
		return 2
	}

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	client := ts.Client()

	// 1. register a LEO satellite via HTTP (exercises real router)
	satBody := `{"name":"SC1","catalog":"9001","a":6800,"e":0.001,"i":51.6,"raan":0,"argp":0,"m":0,"epoch":100000}`
	satID, err := postJSON(client, ts.URL+"/api/satellites", satBody)
	if err != nil {
		fmt.Printf("register sat: %v\n", err)
		return 3
	}
	if satID == "" {
		fmt.Println("register sat: empty id")
		return 4
	}

	stBody := `{"name":"GS1","lat":0,"lon":0,"alt_m":0,"min_elevation":5}`
	stID, err := postJSON(client, ts.URL+"/api/groundstations", stBody)
	if err != nil {
		fmt.Printf("register station: %v\n", err)
		return 5
	}

	// 2. fetch the front-end page to prove it is served.
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		fmt.Printf("page GET: %v\n", err)
		return 6
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("page status: %d\n", resp.StatusCode)
		return 7
	}

	// 3. forecast contacts via HTTP (response is a JSON array of contacts,
	// not an object, so we only check the status code here).
	fcBody := fmt.Sprintf(`{"satellite_id":"%s","station_id":"%s","start":100000,"duration":21600,"step":60}`, satID, stID)
	fcResp, err := client.Post(ts.URL+"/api/contacts/forecast", "application/json", stringReader(fcBody))
	if err != nil {
		fmt.Printf("forecast: %v\n", err)
		return 8
	}
	fcResp.Body.Close()
	if fcResp.StatusCode >= 400 {
		fmt.Printf("forecast http %d\n", fcResp.StatusCode)
		return 8
	}

	// 4. capture the next-window snapshot BEFORE reconcile
	if err := svc.ReconcileAll(ctx); err != nil {
		fmt.Printf("reconcile1: %v\n", err)
		return 9
	}
	snap1, err := svc.SnapshotNextWindows(ctx)
	if err != nil {
		fmt.Printf("snapshot1: %v\n", err)
		return 10
	}

	// 5. reconcile AGAIN and assert idempotent + identical snapshot
	if err := svc.ReconcileAll(ctx); err != nil {
		fmt.Printf("reconcile2: %v\n", err)
		return 11
	}
	snap2, err := svc.SnapshotNextWindows(ctx)
	if err != nil {
		fmt.Printf("snapshot2: %v\n", err)
		return 12
	}
	if !reflect.DeepEqual(snap1, snap2) {
		fmt.Printf("SNAPSHOT MISMATCH after re-reconcile\nsnap1=%+v\nsnap2=%+v\n", snap1, snap2)
		return 13
	}

	// 6. plan a perigee-raise maneuver on a deliberately low-perigee satellite
	// to exercise the maneuver + event path.
	lowBody := `{"name":"LOW","catalog":"9002","a":6600,"e":0.12,"i":28.5,"raan":0,"argp":0,"m":0,"epoch":100000}`
	lowID, err := postJSON(client, ts.URL+"/api/satellites", lowBody)
	if err != nil {
		fmt.Printf("register low: %v\n", err)
		return 14
	}
	mvBody := fmt.Sprintf(`{"satellite_id":"%s","type":"perigee_raise","exec_at":100000}`, lowID)
	if _, err := postJSON(client, ts.URL+"/api/maneuvers", mvBody); err != nil {
		fmt.Printf("plan maneuver: %v\n", err)
		return 15
	}

	// 7. reconcile after maneuver and verify next-window reflects it
	if err := svc.ReconcileAll(ctx); err != nil {
		fmt.Printf("reconcile3: %v\n", err)
		return 16
	}
	snap3, err := svc.SnapshotNextWindows(ctx)
	if err != nil {
		fmt.Printf("snapshot3: %v\n", err)
		return 17
	}
	nw, ok := snap3[lowID]
	if !ok {
		fmt.Printf("low satellite missing from snapshot after maneuver\n")
		return 18
	}
	if nw.NextManeuverEpoch == 0 || nw.NextManeuverType != model.ManeuverPerigeeRaise {
		fmt.Printf("next maneuver not reflected: %+v\n", nw)
		return 19
	}

	fmt.Printf("smoke OK: sat=%s station=%s low=%s snaps=%d\n", satID, stID, lowID, len(snap3))
	return 0
}

// postJSON posts a JSON body and returns the "ID" field of the JSON response.
func postJSON(client *http.Client, url, body string) (string, error) {
	resp, err := client.Post(url, "application/json", stringReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d for %s", resp.StatusCode, url)
	}
	var out map[string]interface{}
	dec := decodeJSONBody(resp.Body)
	if err := dec(&out); err != nil {
		return "", err
	}
	if id, ok := out["ID"].(string); ok {
		return id, nil
	}
	return "", nil
}
