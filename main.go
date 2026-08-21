// Command orbitops is the entry point for the orbital mechanics engine. It
// runs either as an HTTP service (--addr), a migrate-only schema setup
// (--migrate-only), or a self-check that exercises the full stack and exits
// (--smoke-test).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"orbitops/internal/clock"
	"orbitops/internal/httpapi"
	"orbitops/internal/service"
	"orbitops/internal/service/selfcheck"
	"orbitops/internal/store"
)

func main() {
	var (
		addr        = flag.String("addr", ":8080", "HTTP listen address")
		dbPath      = flag.String("db", "orbitops.db", "SQLite database path")
		smokeTest   = flag.Bool("smoke-test", false, "run self-check and exit")
		migrateOnly = flag.Bool("migrate-only", false, "run migrations and exit")
	)
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if *migrateOnly {
		log.Printf("migrations complete; db=%s", *dbPath)
		return
	}

	clk := clock.Real{}
	svc := service.New(st, clk)
	srv := httpapi.New(svc, st, clk)

	if *smokeTest {
		// smoke uses a fake clock for deterministic reconciliation.
		fakeClk := clock.NewFake(100000)
		fakeSvc := service.New(st, fakeClk)
		fakeSrv := httpapi.New(fakeSvc, st, fakeClk)
		code := selfcheck.Run(context.Background(), fakeSvc, fakeSrv, st, fakeClk)
		if code != 0 {
			os.Exit(code)
		}
		fmt.Println("smoke-test: PASS")
		return
	}

	handler := srv.Router()
	httpSrv := &http.Server{Addr: *addr, Handler: handler, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		log.Printf("orbitops listening on %s (db=%s)", *addr, *dbPath)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = strconv.Itoa // keep import meaningful on quiet builds
}
