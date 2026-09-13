package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grabseat/api"
	"grabseat/booking"
	"grabseat/config"
	"grabseat/locking"
	"grabseat/model"
	"grabseat/observability"
	"grabseat/ratelimit"
	"grabseat/store"
	"grabseat/ws"
)

//go:embed db/schema.sql
var schemaSQL string

func main() {
	cfg, err := config.Load()
	log := observability.InitLogger(cfg.LogLevel)
	if err != nil {
		log.Error("config error", "err", err)
		os.Exit(1)
	}

	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "backend"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracer, err := observability.InitTracer(ctx, cfg.OTelExporter, instanceID)
	if err != nil {
		log.Error("tracer init failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdownTracer(sctx)
	}()

	// --- Postgres: durable state. Retry briefly so we tolerate the DB coming up
	// a moment after us even though compose already waits for it to be healthy.
	st, err := connectStore(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.RunMigrations(ctx, schemaSQL); err != nil {
		log.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	eventID, err := st.SeedIfEmpty(ctx)
	if err != nil {
		log.Error("seed failed", "err", err)
		os.Exit(1)
	}
	event, err := st.PrimaryEvent(ctx)
	if err != nil {
		log.Error("load event failed", "err", err)
		os.Exit(1)
	}
	seats, err := st.Seats(ctx, eventID)
	if err != nil {
		log.Error("load seats failed", "err", err)
		os.Exit(1)
	}
	log.Info("venue ready", "event", event.Name, "seats", len(seats), "instance", instanceID)

	// --- Redis: ephemeral locks + pub/sub. Not fatal if it's briefly down at
	// boot; the service still serves booked seats and recovers when Redis returns.
	lock := locking.New(cfg.RedisAddr, cfg.RedisPassword, log)
	defer lock.Close()
	rehydrateBooked(ctx, st, lock, eventID, log)

	hub := ws.NewHub(log)
	holdTTL := time.Duration(cfg.HoldTTLSeconds) * time.Second
	bookings := booking.NewService(st, lock, eventID, instanceID, holdTTL)
	limiter := ratelimit.New(cfg.RateLimitPerMin)

	srv := &api.Server{
		Store:         st,
		Lock:          lock,
		Bookings:      bookings,
		Hub:           hub,
		Limiter:       limiter,
		Event:         event,
		Seats:         seats,
		InstanceID:    instanceID,
		HoldTTL:       holdTTL,
		AllowedOrigin: cfg.AllowedOrigin,
	}

	// Every published seat event (from any instance) is fanned out to this
	// instance's connected clients. This is the cross-instance half of the design.
	go lock.SubscribeSeatEvents(ctx, func(ev model.SeatEvent) {
		if b, err := json.Marshal(ev); err == nil {
			hub.Broadcast(b)
		}
	})

	// Expired holds arrive as keyspace events on every instance; each one frees
	// the seat for its own clients. No polling, no cleanup job.
	go lock.SubscribeExpiries(ctx, func(seatID int) {
		ev := model.SeatEvent{
			Type:     model.MsgFreed,
			SeatID:   seatID,
			Status:   model.StatusAvailable,
			ServedBy: instanceID,
		}
		if b, err := json.Marshal(ev); err == nil {
			hub.Broadcast(b)
		}
		log.Debug("hold expired, seat freed", "seat_id", seatID)
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("listening", "port", cfg.Port, "instance", instanceID)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func connectStore(ctx context.Context, url string, log logger) (*store.Store, error) {
	var lastErr error
	for i := 0; i < 15; i++ {
		st, err := store.New(ctx, url)
		if err == nil {
			return st, nil
		}
		lastErr = err
		log.Info("waiting for postgres", "attempt", i+1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}

func rehydrateBooked(ctx context.Context, st *store.Store, lock *locking.Client, eventID int, log logger) {
	booked, err := st.BookedSeats(ctx, eventID)
	if err != nil {
		log.Warn("could not load booked seats for rehydrate", "err", err)
		return
	}
	if err := lock.RehydrateBooked(ctx, booked); err != nil {
		log.Warn("redis rehydrate skipped (redis down?)", "err", err)
		return
	}
	if len(booked) > 0 {
		log.Info("rehydrated booked seats into redis", "count", len(booked))
	}
}

// logger is the small slice of *slog.Logger the helpers above need.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}
