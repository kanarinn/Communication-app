package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/kanarinn/Communication-app/services/gateway/internal/token"
)

func main() {
	authProxy := newProxy(mustEnv("AUTH_URL"))
	chatProxy := newProxy(mustEnv("CHAT_URL"))
	secret := []byte(mustEnv("JWT_SECRET"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	route(mux, "/api/auth", stripAPI(authProxy))

	route(mux, "/api/users", requireAuth(secret, stripAPI(authProxy)))
	route(mux, "/api/friends", requireAuth(secret, stripAPI(chatProxy)))
	route(mux, "/api/messages", requireAuth(secret, stripAPI(chatProxy)))

	mux.Handle("/ws", requireAuth(secret, chatProxy))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("gateway listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, sanitize(mux)))
}

func route(mux *http.ServeMux, prefix string, h http.Handler) {
	mux.Handle(prefix, h)
	mux.Handle(prefix+"/", h)
}

func stripAPI(h http.Handler) http.Handler { return http.StripPrefix("/api", h) }

func sanitize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-User-ID")
		r.Header.Del("X-Username")
		next.ServeHTTP(w, r)
	})
}

func requireAuth(secret []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r)
		if raw == "" {
			raw = r.URL.Query().Get("token")
		}
		claims, err := token.Verify(secret, raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		r.Header.Set("X-User-ID", claims.Sub)
		r.Header.Set("X-Username", claims.Username)
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return h[7:]
	}
	return ""
}

func newProxy(raw string) *httputil.ReverseProxy {
	target, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("bad upstream url %q: %v", raw, err)
	}
	p := httputil.NewSingleHostReverseProxy(target)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy %s %s -> %s: %v", r.Method, r.URL.Path, target.Host, err)
		writeError(w, http.StatusBadGateway, "upstream unavailable")
	}
	return p
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}
