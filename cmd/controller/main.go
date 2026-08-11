package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/audit"
	"github.com/RenAhsAcme/SCWebVPN/internal/binding"
	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
	"github.com/RenAhsAcme/SCWebVPN/internal/config"
	"github.com/RenAhsAcme/SCWebVPN/internal/controller"
	"github.com/RenAhsAcme/SCWebVPN/internal/httpapi"
	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/presence"
	"github.com/RenAhsAcme/SCWebVPN/internal/session"
	"github.com/RenAhsAcme/SCWebVPN/internal/signaling"
	"github.com/RenAhsAcme/SCWebVPN/internal/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("Controller stopped", "error", fmt.Sprintf("%T", err))
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("controller", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "Controller JSON config path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("controller requires -config")
	}
	cfg, err := config.LoadController(*configPath)
	if err != nil {
		return err
	}
	secret, err := config.ReadSecret(cfg.InternalAuthSecretFile)
	if err != nil {
		return err
	}
	browserAuth, err := httpapi.NewBrowserAuth(secret)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, store, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	handler, err := controller.New(controller.Dependencies{
		BrowserAuth: browserAuth, Bindings: binding.NewService(store), Catalog: store,
		CatalogAdmin: catalog.NewManager(store), Sessions: session.NewService(store),
		Challenges: identity.NewChallengeIssuer(store), AgentAuth: identity.NewAgentAuthenticator(store),
		Signals: signaling.NewBroker(), STUNURLs: cfg.STUNURLs,
		Audit: audit.NewRecorder(store), Presence: presence.NewService(store),
		PublicBaseURL: cfg.PublicBaseURL,
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10,
	}
	serveError := make(chan error, 1)
	go func() {
		slog.Info("Controller ready")
		serveError <- server.Serve(listener)
	}()
	select {
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
