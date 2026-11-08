// Package app wires the hub's components together and runs them until the
// context is canceled. Wiring is explicit on purpose (docs/adr/0010).
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/xonoko/robust-home-automation/internal/api"
	"github.com/xonoko/robust-home-automation/internal/bus"
	"github.com/xonoko/robust-home-automation/internal/config"
	"github.com/xonoko/robust-home-automation/internal/store"
	"github.com/xonoko/robust-home-automation/internal/telemetry"
	"github.com/xonoko/robust-home-automation/internal/version"
	"github.com/xonoko/robust-home-automation/internal/webui"
)

// Run starts the hub and blocks until ctx is canceled and shutdown completes.
func Run(ctx context.Context, cfg config.Config, logOut io.Writer) (err error) {
	tel, err := telemetry.Setup(ctx, cfg.Telemetry, cfg.Node.ID, logOut)
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	log := tel.Logger
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if serr := tel.Shutdown(sctx); serr != nil {
			log.Warn("telemetry shutdown", "error", serr)
		}
	}()

	log.Info("starting hub", "commit", version.Commit, "http_addr", cfg.HTTP.Addr)

	busClient, err := bus.Connect(cfg.NATS, "hub/"+cfg.Node.ID, log)
	if err != nil {
		return err
	}
	checks := []api.Check{{Name: "nats", Critical: true, Fn: busClient.Check}}

	if cfg.Database.URL == "" {
		log.Warn("database.url is not set: running without Postgres")
	} else {
		st, err := store.Open(cfg.Database.URL)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		// Not critical: the hot path never depends on Postgres (docs/adr/0004).
		checks = append(checks, api.Check{Name: "postgres", Fn: st.Check})
	}

	srv := api.New(api.Options{Checks: checks, Metrics: tel.MetricsHandler, UI: webui.Handler()})
	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr, err)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		return shutdown(log, cfg.Shutdown, srv, httpServer, busClient)
	})

	err = g.Wait()
	log.Info("hub stopped", "error", err)
	return err
}

// shutdown reports "not ready" for the drain delay so new work goes elsewhere,
// then stops the HTTP server and drains the NATS connection.
func shutdown(log *slog.Logger, cfg config.Shutdown, srv *api.Server, httpServer *http.Server, busClient *bus.Client) error {
	log.Info("shutting down", "drain_delay", cfg.DrainDelay)
	srv.SetShuttingDown()
	time.Sleep(cfg.DrainDelay)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout-cfg.DrainDelay)
	defer cancel()

	var errs []error
	if err := httpServer.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	if err := busClient.Drain(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
