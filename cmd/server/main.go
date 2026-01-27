package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"event-inbox/internal/crypto"
	"event-inbox/internal/db"
	"event-inbox/internal/ingress"
	"event-inbox/internal/storage"
)

func main() {
	startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	maxBytes := int64(1048576)
	if v := os.Getenv("MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBytes = n
		}
	}

	enc, err := crypto.NewEncryptorFromB64(os.Getenv("PAYLOAD_ENC_KEY_B64"))
	if err != nil {
		log.Fatalf("encryptor init failed: %v", err)
	}

	s3, err := storage.New(storage.Config{
		Endpoint:  os.Getenv("S3_ENDPOINT"),
		Region:    os.Getenv("S3_REGION"),
		Bucket:    os.Getenv("S3_BUCKET"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
	})
	if err != nil {
		log.Fatalf("s3 init failed: %v", err)
	}

	log.Println("connecting to database...")
	pool, err := db.Connect(startCtx, databaseURL)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer pool.Close()

	log.Println("running migrations...")
	if err := db.RunMigrations(context.Background(), pool); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	handler := &ingress.Handler{
		DB:        pool,
		S3:        s3,
		Encryptor: enc,
		MaxBytes:  maxBytes,
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	ctx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx)
	log.Println("bye")
}
