package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/version"
	"github.com/ipchronicle/ipchronicle/internal/webui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(arguments []string) error {
	if len(arguments) == 1 {
		switch arguments[0] {
		case "version", "--version":
			fmt.Println(version.Value)
			return nil
		case "healthcheck":
			return healthcheck()
		}
	}
	if len(arguments) >= 2 && arguments[0] == "admin" {
		return runAdministratorCommand(arguments[1:])
	}
	if len(arguments) != 0 {
		return errors.New("usage: ipchronicle-center [version|healthcheck|admin reset-password --password-stdin|admin disable-totp]")
	}
	return serve()
}

func serve() error {
	configuration, err := center.LoadRuntimeConfig()
	if err != nil {
		return err
	}
	store, err := database.Open(context.Background(), configuration.DatabasePaths)
	if err != nil {
		return err
	}
	defer store.Close()
	administrator := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := administrator.Bootstrap(context.Background(), configuration.AdminUsername, configuration.AdminPassword); err != nil {
		return err
	}
	nodeService := nodes.NewService(store.Config, store.ConfigQueries, store.MasterKey)

	server := &http.Server{
		Addr: configuration.ListenAddress,
		Handler: center.NewHTTPHandler(center.HTTPOptions{
			Version:        version.Value,
			Web:            webui.Handler(),
			Administrator:  administrator,
			Nodes:          nodeService,
			Store:          store,
			ExternalOrigin: configuration.ExternalOrigin,
			TrustedProxies: configuration.TrustedProxies,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("IPChronicle center %s listening on %s", version.Value, configuration.ListenAddress)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runAdministratorCommand(arguments []string) error {
	paths, err := center.LoadDatabasePaths()
	if err != nil {
		return err
	}
	store, err := database.OpenConfigurationForRecovery(context.Background(), paths)
	if err != nil {
		return err
	}
	defer store.Close()
	administrator := admin.NewService(store.Database, store.Queries, store.MasterKey)
	if len(arguments) == 1 && arguments[0] == "disable-totp" {
		if err := administrator.RecoverDisableTOTP(context.Background()); err != nil {
			return err
		}
		fmt.Println("administrator TOTP disabled and all sessions revoked")
		return nil
	}
	if len(arguments) == 2 && arguments[0] == "reset-password" && arguments[1] == "--password-stdin" {
		input, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		password := strings.TrimSuffix(strings.TrimSuffix(string(input), "\n"), "\r")
		if err := administrator.RecoverPassword(context.Background(), password); err != nil {
			return err
		}
		fmt.Println("administrator password reset and all sessions revoked")
		return nil
	}
	return errors.New("usage: ipchronicle-center admin reset-password --password-stdin | admin disable-totp")
}

func healthcheck() error {
	url := os.Getenv("IPCHRONICLE_HEALTHCHECK_URL")
	if url == "" {
		url = "http://127.0.0.1:8080/healthz"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}
