package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kanarinn/Communication-app/services/chat/internal/authclient"
	"github.com/kanarinn/Communication-app/services/chat/internal/httpapi"
	"github.com/kanarinn/Communication-app/services/chat/internal/realtime"
	"github.com/kanarinn/Communication-app/services/chat/internal/store"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer pool.Close()
	st := store.New(pool)
	if err := retry(ctx, 10, func() error { return st.Migrate(ctx) }); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mustEnv("REDIS_ADDR")})
	if err := retry(ctx, 10, func() error { return rdb.Ping(ctx).Err() }); err != nil {
		log.Fatalf("redis: %v", err)
	}

	hub := realtime.NewHub(rdb, st)
	go func() {
		if err := hub.Run(ctx); err != nil && ctx.Err() == nil {
			log.Fatalf("hub stopped: %v", err)
		}
	}()

	srv := httpapi.New(st, authclient.New(mustEnv("AUTH_URL")), hub)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("chat listening on :%s", port)
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
