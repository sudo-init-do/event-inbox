package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"event-inbox/internal/db"
)

func main() {
	// Give containers time to become ready.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	log.Println("connecting to database...")
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer pool.Close()

	log.Println("running migrations...")
	if err := db.RunMigrations(context.Background(), pool); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	log.Println("database connected and migrations applied")

	// Stay alive until Ctrl+C / docker stop
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down")
}
