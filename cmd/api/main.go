package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"budol/gmr-engine/internal/config"
	"budol/gmr-engine/internal/deployer"
	"budol/gmr-engine/internal/httpapi"
	"budol/gmr-engine/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	engineStore, err := store.NewMemgraphStore(ctx, cfg.MemgraphURI, cfg.MemgraphUser, cfg.MemgraphPassword)
	if err != nil {
		log.Fatalf("engine memgraph connection error: %v", err)
	}
	defer engineStore.Close(context.Background())

	if _, err := engineStore.SeedOwnerAccount(ctx, cfg.OwnerUsername, cfg.OwnerEmail, httpapi.HashPassword(cfg.OwnerPassword), cfg.OwnerName); err != nil {
		log.Fatalf("owner account seed error: %v", err)
	}

	runCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	if cfg.DeployerEnabled {
		contractDeployer, err := deployer.New(ctx, cfg, engineStore)
		if err != nil {
			log.Fatalf("contract deployer error: %v", err)
		}
		defer contractDeployer.Close()
		go contractDeployer.Run(runCtx)
		log.Printf("contract deployer worker started")
	}

	app := httpapi.New(cfg, engineStore)

	errs := make(chan error, 1)
	go func() {
		errs <- app.Listen(cfg.Addr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		log.Fatalf("server error: %v", err)
	case <-quit:
		stopWorkers()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}
}
