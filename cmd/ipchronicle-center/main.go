package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center"
	"github.com/ipchronicle/ipchronicle/internal/version"
	"github.com/ipchronicle/ipchronicle/internal/webui"
)

const defaultListenAddress = ":8080"

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version", "--version":
			fmt.Println(version.Value)
			return
		case "healthcheck":
			if err := healthcheck(); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	if err := serve(); err != nil {
		log.Fatal(err)
	}
}

func serve() error {
	listenAddress := os.Getenv("IPCHRONICLE_LISTEN_ADDRESS")
	if listenAddress == "" {
		listenAddress = defaultListenAddress
	}

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           center.NewHTTPHandler(version.Value, webui.Handler()),
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

	log.Printf("IPChronicle center %s listening on %s", version.Value, listenAddress)
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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
