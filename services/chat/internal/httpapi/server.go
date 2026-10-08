package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kanarinn/Communication-app/services/chat/internal/authclient"
	"github.com/kanarinn/Communication-app/services/chat/internal/realtime"
	"github.com/kanarinn/Communication-app/services/chat/internal/store"
)

type Server struct {
	store *store.Store
	auth  *authclient.Client
	hub   *realtime.Hub
	mux   *http.ServeMux
}

func New(st *store.Store, auth *authclient.Client, hub *realtime.Hub) *Server {
	s := &Server{store: st, auth: auth, hub: hub, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	s.mux.HandleFunc("GET /friends", s.handleFriends)
	s.mux.HandleFunc("POST /friends/requests", s.handleSendRequest)
	s.mux.HandleFunc("POST /friends/requests/{id}/accept", s.respond(true))
	s.mux.HandleFunc("POST /friends/requests/{id}/decline", s.respond(false))
	s.mux.HandleFunc("GET /messages/{userID}", s.handleMessages)
	s.mux.HandleFunc("GET /ws", hub.ServeWS)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

type requestView struct {
	ID        int64           `json:"id"`
	User      authclient.User `json:"user"` // the other user
	CreatedAt time.Time       `json:"createdAt"`
}

type friendsResponse struct {
	Friends  []authclient.User `json:"friends"`
	Incoming []requestView     `json:"incoming"`
	Outgoing []requestView     `json:"outgoing"`
}

func (s *Server) handleFriends(w http.ResponseWriter, r *http.Request) {
	me := callerID(r)
	ctx := r.Context()

	friendIDs, err := s.store.FriendIDs(ctx, me)
	if err != nil {
		s.dbError(w, err)
		return
	}
	incoming, outgoing, err := s.store.PendingRequests(ctx, me)
	if err != nil {
		s.dbError(w, err)
		return
	}

	ids := append([]string{}, friendIDs...)
	for _, q := range incoming {
		ids = append(ids, q.FromUser)
	}
	for _, q := range outgoing {
		ids = append(ids, q.ToUser)
	}
	users, err := s.auth.ByIDs(ctx, ids)
	if err != nil {
		log.Printf("friends: %v", err)
		writeError(w, http.StatusBadGateway, "auth service unavailable")
		return
	}

	resp := friendsResponse{Friends: []authclient.User{}, Incoming: []requestView{}, Outgoing: []requestView{}}
	for _, id := range friendIDs {
		resp.Friends = append(resp.Friends, users[id])
	}
	for _, q := range incoming {
		resp.Incoming = append(resp.Incoming, requestView{ID: q.ID, User: users[q.FromUser], CreatedAt: q.CreatedAt})
	}
	for _, q := range outgoing {
		resp.Outgoing = append(resp.Outgoing, requestView{ID: q.ID, User: users[q.ToUser], CreatedAt: q.CreatedAt})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSendRequest(w http.ResponseWriter, r *http.Request) {
	me := callerID(r)
	var in struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	target, err := s.auth.ByUsername(r.Context(), strings.TrimSpace(in.Username))
	switch {
	case errors.Is(err, authclient.ErrNotFound):
		writeError(w, http.StatusNotFound, "no user with that username")
		return
	case err != nil:
		log.Printf("send request: %v", err)
		writeError(w, http.StatusBadGateway, "auth service unavailable")
		return
	case target.ID == me:
		writeError(w, http.StatusBadRequest, "you cannot befriend yourself")
		return
	}
	req, err := s.store.SendRequest(r.Context(), me, target.ID)
	switch {
	case errors.Is(err, store.ErrAlreadyFriends):
		writeError(w, http.StatusConflict, "already friends")
		return
	case errors.Is(err, store.ErrAlreadyRequested):
		writeError(w, http.StatusConflict, "request already sent")
		return
	case err != nil:
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) respond(accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid request id")
			return
		}
		err = s.store.Respond(r.Context(), id, callerID(r), accept)
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "no pending request with that id")
			return
		case err != nil:
			s.dbError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	me := callerID(r)
	peer := r.PathValue("userID")

	friends, err := s.store.AreFriends(r.Context(), me, peer)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !friends {
		writeError(w, http.StatusForbidden, "you can only read conversations with friends")
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	msgs, err := s.store.Messages(r.Context(), me, peer, before, limit)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func callerID(r *http.Request) string { return r.Header.Get("X-User-ID") }

func (s *Server) dbError(w http.ResponseWriter, err error) {
	log.Printf("db: %v", err)
	writeError(w, http.StatusInternalServerError, "database error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
