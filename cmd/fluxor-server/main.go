package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/config"
	"github.com/magicorntech/Fluxor/internal/httpapi"
	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/store/memory"
	mongostore "github.com/magicorntech/Fluxor/internal/store/mongo"
	"github.com/magicorntech/Fluxor/internal/version"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config.Load()

	st, closeStore := openStore(log, cfg)
	defer closeStore()

	authSvc := newAuthService(log, cfg, st)
	bootstrapAdmin(log, cfg, authSvc)
	az := authz.New(st)
	grantBootstrapAdmin(log, az)

	proxies, err := httpapi.ParseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		log.Error("invalid FLUXOR_TRUSTED_PROXIES", "err", err)
		os.Exit(1)
	}
	handler := httpapi.New(httpapi.Options{
		Store:          st,
		Auth:           authSvc,
		Authz:          az,
		Logger:         log,
		CORSOrigins:    cfg.CORSOrigins,
		CookieSecure:   cfg.CookieSecure,
		TrustedProxies: proxies,
		// In the config 0 means "off"; httpapi uses a negative number for that and 0 for "default".
		LoginPerMinute:   limitOrOff(cfg.LoginPerMinute),
		RefreshPerMinute: limitOrOff(cfg.RefreshPerMinute),
	})
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("fluxor-server listening", "addr", cfg.Addr, "version", version.Version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			closeStore()
			os.Exit(1)
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown failed", "err", err)
			closeStore()
			os.Exit(1)
		}
	}
}

// openStore connects to MongoDB when FLUXOR_MONGO_URI is set and otherwise falls back to the
// in-memory store. The returned func releases the store and is safe to call more than once.
func openStore(log *slog.Logger, cfg config.Config) (store.Store, func()) {
	if cfg.MongoURI == "" {
		log.Warn("FLUXOR_MONGO_URI is not set: using the in-memory store, data is lost on restart")
		return memory.New(), func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ms, err := mongostore.Connect(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		// The URI may contain credentials, so only the error is logged.
		log.Error("cannot connect to MongoDB", "err", err)
		os.Exit(1)
	}
	log.Info("connected to MongoDB", "db", cfg.MongoDB)

	closed := false
	return ms, func() {
		if closed {
			return
		}
		closed = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ms.Close(ctx); err != nil {
			log.Error("closing MongoDB", "err", err)
		}
	}
}

func newAuthService(log *slog.Logger, cfg config.Config, st store.Store) *auth.Service {
	secret := []byte(cfg.JWTSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Error("cannot generate a token secret", "err", err)
			os.Exit(1)
		}
		log.Warn("FLUXOR_JWT_SECRET is not set: generated a random one, all sessions end when the server restarts")
	}
	svc, err := auth.NewService(auth.Options{Users: st, Sessions: st, Tokens: st, Secret: secret, Logger: log})
	if err != nil {
		log.Error("cannot set up authentication", "err", err)
		os.Exit(1)
	}
	return svc
}

// bootstrapAdmin creates the first administrator on an empty database.
func bootstrapAdmin(log *slog.Logger, cfg config.Config, svc *auth.Service) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := svc.Bootstrap(ctx, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		log.Error("cannot create the initial administrator", "err", err)
		os.Exit(1)
	}
	if !res.Created {
		return
	}
	if res.Password != "" {
		// Printed exactly once; it cannot be recovered later. The account must change it on first login.
		log.Warn("created the initial administrator with a generated password; it is shown only now and must be changed on first login",
			"username", res.Username, "password", res.Password)
		return
	}
	log.Info("created the initial administrator from FLUXOR_ADMIN_*; the password must be changed on first login", "username", res.Username)
}

// grantBootstrapAdmin makes the account created by the first-run setup an administrator. It only
// acts while no administrator exists, so it also repairs databases created before roles existed.
func grantBootstrapAdmin(log *slog.Logger, az *authz.Authorizer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	g, err := az.EnsureBootstrapAdmin(ctx)
	if err != nil {
		log.Error("cannot grant the administrator role", "err", err)
		os.Exit(1)
	}
	if g.Created {
		log.Info("granted the administrator role to the initial account", "username", g.Username)
	}
}

func limitOrOff(perMinute int) int {
	if perMinute == 0 {
		return -1
	}
	return perMinute
}
