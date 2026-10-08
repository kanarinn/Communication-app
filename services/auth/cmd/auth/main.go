package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kanarinn/Communication-app/services/auth/internal/httpapi"
	"github.com/kanarinn/Communication-app/services/auth/internal/users"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer pool.Close()

	store := users.NewStore(pool)
	if err := retry(ctx, 10, func() error { return store.Migrate(ctx) }); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	srv := httpapi.New(store, []byte(mustEnv("JWT_SECRET")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("auth listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, srv))
}

func retry(ctx context.Context, attempts int, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		log.Printf("attempt %d/%d failed: %v", i+1, attempts, err)
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}
