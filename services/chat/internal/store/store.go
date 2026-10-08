package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Request struct {
	ID        int64     `json:"id"`
	FromUser  string    `json:"fromUser"`
	ToUser    string    `json:"toUser"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type Message struct {
	ID          int64     `json:"id"`
	SenderID    string    `json:"senderId"`
	RecipientID string    `json:"recipientId"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"createdAt"`
}

var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyFriends   = errors.New("already friends")
	ErrAlreadyRequested = errors.New("request already sent")
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS friend_requests (
			id         bigserial PRIMARY KEY,
			from_user  uuid NOT NULL,
			to_user    uuid NOT NULL,
			status     text NOT NULL DEFAULT 'pending'
			           CHECK (status IN ('pending', 'accepted', 'declined')),
			created_at timestamptz NOT NULL DEFAULT now(),
			CHECK (from_user <> to_user)
		);

		CREATE UNIQUE INDEX IF NOT EXISTS friend_requests_pair_idx
			ON friend_requests (LEAST(from_user, to_user), GREATEST(from_user, to_user));

		CREATE TABLE IF NOT EXISTS messages (
			id           bigserial PRIMARY KEY,
			sender_id    uuid NOT NULL,
			recipient_id uuid NOT NULL,
			body         text NOT NULL,
			created_at   timestamptz NOT NULL DEFAULT now()
		);

		CREATE INDEX IF NOT EXISTS messages_pair_idx
			ON messages (LEAST(sender_id, recipient_id), GREATEST(sender_id, recipient_id), id);
	`)
	return err
}

func (s *Store) FriendIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT CASE WHEN from_user::text = $1 THEN to_user::text ELSE from_user::text END
		FROM friend_requests
		WHERE status = 'accepted' AND (from_user::text = $1 OR to_user::text = $1)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) PendingRequests(ctx context.Context, userID string) (incoming, outgoing []Request, err error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, from_user::text, to_user::text, status, created_at
		FROM friend_requests
		WHERE status = 'pending' AND (from_user::text = $1 OR to_user::text = $1)
		ORDER BY created_at`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	incoming, outgoing = []Request{}, []Request{}
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.FromUser, &r.ToUser, &r.Status, &r.CreatedAt); err != nil {
			return nil, nil, err
		}
		if r.ToUser == userID {
			incoming = append(incoming, r)
		} else {
			outgoing = append(outgoing, r)
		}
	}
	return incoming, outgoing, rows.Err()
}

func (s *Store) SendRequest(ctx context.Context, from, to string) (Request, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)

	var existing Request
	err = tx.QueryRow(ctx, `
		SELECT id, from_user::text, to_user::text, status, created_at
		FROM friend_requests
		WHERE (from_user::text = $1 AND to_user::text = $2)
		   OR (from_user::text = $2 AND to_user::text = $1)
		FOR UPDATE`, from, to,
	).Scan(&existing.ID, &existing.FromUser, &existing.ToUser, &existing.Status, &existing.CreatedAt)

	var out Request
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = tx.QueryRow(ctx, `
			INSERT INTO friend_requests (from_user, to_user) VALUES ($1::uuid, $2::uuid)
			RETURNING id, from_user::text, to_user::text, status, created_at`, from, to,
		).Scan(&out.ID, &out.FromUser, &out.ToUser, &out.Status, &out.CreatedAt)
	case err != nil:
		return Request{}, err
	case existing.Status == "accepted":
		return Request{}, ErrAlreadyFriends
	case existing.Status == "pending" && existing.FromUser == from:
		return Request{}, ErrAlreadyRequested
	case existing.Status == "pending": // they asked first: accept
		err = tx.QueryRow(ctx, `
			UPDATE friend_requests SET status = 'accepted' WHERE id = $1
			RETURNING id, from_user::text, to_user::text, status, created_at`, existing.ID,
		).Scan(&out.ID, &out.FromUser, &out.ToUser, &out.Status, &out.CreatedAt)
	default: // declined: start over in the new direction
		err = tx.QueryRow(ctx, `
			UPDATE friend_requests
			SET status = 'pending', from_user = $2::uuid, to_user = $3::uuid, created_at = now()
			WHERE id = $1
			RETURNING id, from_user::text, to_user::text, status, created_at`, existing.ID, from, to,
		).Scan(&out.ID, &out.FromUser, &out.ToUser, &out.Status, &out.CreatedAt)
	}
	if err != nil {
		return Request{}, err
	}
	return out, tx.Commit(ctx)
}

func (s *Store) Respond(ctx context.Context, requestID int64, userID string, accept bool) error {
	status := "declined"
	if accept {
		status = "accepted"
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE friend_requests SET status = $3
		WHERE id = $1 AND to_user::text = $2 AND status = 'pending'`, requestID, userID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AreFriends(ctx context.Context, a, b string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM friend_requests
			WHERE status = 'accepted'
			  AND ((from_user::text = $1 AND to_user::text = $2)
			    OR (from_user::text = $2 AND to_user::text = $1)))`, a, b).Scan(&ok)
	return ok, err
}

func (s *Store) SaveMessage(ctx context.Context, sender, recipient, body string) (Message, error) {
	var m Message
	err := s.db.QueryRow(ctx, `
		INSERT INTO messages (sender_id, recipient_id, body) VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id, sender_id::text, recipient_id::text, body, created_at`, sender, recipient, body,
	).Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &m.CreatedAt)
	return m, err
}

func (s *Store) Messages(ctx context.Context, a, b string, beforeID int64, limit int) ([]Message, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, sender_id::text, recipient_id::text, body, created_at
		FROM messages
		WHERE ((sender_id::text = $1 AND recipient_id::text = $2)
		    OR (sender_id::text = $2 AND recipient_id::text = $1))
		  AND ($3::bigint = 0 OR id < $3::bigint)
		ORDER BY id DESC
		LIMIT $4`, a, b, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse: we fetched newest-first to apply LIMIT, but display oldest-first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if out == nil {
		out = []Message{}
	}
	return out, nil
}
