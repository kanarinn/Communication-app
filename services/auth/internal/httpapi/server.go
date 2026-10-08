package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/kanarinn/Communication-app/services/auth/internal/token"
	"github.com/kanarinn/Communication-app/services/auth/internal/users"
)

const tokenTTL = 7 * 24 * time.Hour

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

type Server struct {
	store  *users.Store
	secret []byte
	mux    *http.ServeMux
}

func New(store *users.Store, secret []byte) *Server {
	s := &Server{store: store, secret: secret, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	s.mux.HandleFunc("POST /auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /users/@me", s.handleMe)

	s.mux.HandleFunc("GET /internal/users/by-username/{username}", s.handleInternalByUsername)
	s.mux.HandleFunc("GET /internal/users", s.handleInternalByIDs)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  users.User `json:"user"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernameRe.MatchString(in.Username) {
		writeError(w, http.StatusBadRequest, "username must be 3-32 letters, digits or underscores")
		return
	}
	if len(in.Password) < 6 || len(in.Password) > 72 { // 72 is bcrypt's input limit
		writeError(w, http.StatusBadRequest, "password must be 6-72 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.store.Create(r.Context(), in.Username, string(hash))
	switch {
	case errors.Is(err, users.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username already taken")
		return
	case err != nil:
		log.Printf("register: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.issue(w, http.StatusCreated, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, hash, err := s.store.ByUsername(r.Context(), strings.TrimSpace(in.Username))
	if err != nil && !errors.Is(err, users.ErrNotFound) {
		log.Printf("login: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.issue(w, http.StatusOK, u)
}

func (s *Server) issue(w http.ResponseWriter, status int, u users.User) {
	tok, err := token.Sign(s.secret, token.Claims{
		Sub:      u.ID,
		Username: u.Username,
		Exp:      time.Now().Add(tokenTTL).Unix(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not sign token")
		return
	}
	writeJSON(w, status, authResponse{Token: tok, User: u})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-User-ID")
	if id == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := s.store.ByID(r.Context(), id)
	if errors.Is(err, users.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleInternalByUsername(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.store.ByUsername(r.Context(), r.PathValue("username"))
	if errors.Is(err, users.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleInternalByIDs(w http.ResponseWriter, r *http.Request) {
	var ids []string
	if raw := r.URL.Query().Get("ids"); raw != "" {
		ids = strings.Split(raw, ",")
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, []users.User{})
		return
	}
	list, err := s.store.ByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if list == nil {
		list = []users.User{}
	}
	writeJSON(w, http.StatusOK, list)
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
