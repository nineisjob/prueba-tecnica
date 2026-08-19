// Command api wires together config -> pool -> migrate -> seed -> repos ->
// hub -> manager -> recover -> router -> server -> graceful shutdown. This
// file is intentionally just wiring: every real decision lives in the
// package it wires (SRP).
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/geferson/bidcraft/backend/internal/auction"
	"github.com/geferson/bidcraft/backend/internal/auth"
	"github.com/geferson/bidcraft/backend/internal/domain"
	"github.com/geferson/bidcraft/backend/internal/platform/config"
	"github.com/geferson/bidcraft/backend/internal/platform/db"
	"github.com/geferson/bidcraft/backend/internal/platform/hash"
	"github.com/geferson/bidcraft/backend/internal/platform/jwt"
	"github.com/geferson/bidcraft/backend/internal/platform/logx"
	"github.com/geferson/bidcraft/backend/internal/platform/seed"
	"github.com/geferson/bidcraft/backend/internal/repository/postgres"
	bidcrafthttp "github.com/geferson/bidcraft/backend/internal/transport/http"
	"github.com/geferson/bidcraft/backend/internal/transport/ws"
	"github.com/geferson/bidcraft/backend/migrations"
)

func main() {
	// Distroless/scratch runtime images have no curl/wget/shell, so
	// docker-compose's healthcheck runs this binary against itself instead.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthcheck())
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logx.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if cfg.MigrateOnStart {
		if err := db.Migrate(cfg.DatabaseURL, migrations.FS); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		log.Info("migrations applied")
	}

	hasher := hash.NewBcryptHasher()

	if cfg.SeedDemoData {
		if err := seed.Run(ctx, pool, hasher, log); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
	}

	auctionRepo := postgres.NewAuctionRepo(pool)
	bidRepo := postgres.NewBidRepo(pool)
	userRepo := postgres.NewUserRepo(pool)

	tokens := jwt.NewService(cfg.JWTSecret, cfg.JWTTTLHours)
	authSvc := auth.NewService(userRepo, hasher, tokens)

	hub := ws.NewHub(log)
	manager := auction.NewManager(auctionRepo, hub, domain.RealClock{}, log)

	// Recover BEFORE the HTTP listener starts, so no request can reach a
	// not-yet-recovered auction.
	if err := manager.Recover(ctx); err != nil {
		return fmt.Errorf("recover open auctions: %w", err)
	}
	log.Info("auction rooms recovered")

	authHandler := bidcrafthttp.NewAuthHandler(authSvc)
	auctionHandler := bidcrafthttp.NewAuctionHandler(auctionRepo, auctionRepo, manager, manager, manager, bidRepo, userRepo)
	wsHandler := ws.NewHandler(hub, manager, bidRepo, userRepo, cfg.CORSAllowedOrigins, log)

	router := bidcrafthttp.NewRouter(bidcrafthttp.RouterDeps{
		Auth:          authHandler,
		Auctions:      auctionHandler,
		TokenVerifier: tokens,
		Pinger:        pingerFunc(pool.Ping),
		Log:           log,
		CORSOrigins:   cfg.CORSAllowedOrigins,
		WS:            wsHandler,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http server shutdown error", "err", err)
	}
	if err := manager.Shutdown(shutdownCtx); err != nil {
		log.Error("auction manager shutdown error", "err", err)
	}
	return nil
}

type pingerFunc func(ctx context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
