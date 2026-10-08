package users

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	ErrNotFound      = errors.New("user not found")
	ErrUsernameTaken = errors.New("username already taken")
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			username      text UNIQUE NOT NULL,
			password_hash text NOT NULL,
			created_at    timestamptz NOT NULL DEFAULT now()
		)`)
	return err
}

func (s *Store) Create(ctx context.Context, username, passwordHash string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash) VALUES ($1, $2)
		RETURNING id::text, username, created_at`, username, passwordHash,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return User{}, ErrUsernameTaken
	}
	return u, err
}

func (s *Store) ByUsername(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, username, created_at, password_hash FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) ByID(ctx context.Context, id string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT id::text, username, created_at FROM users WHERE id::text = $1`, id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) ByIDs(ctx context.Context, ids []string) ([]User, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, username, created_at FROM users WHERE id::text = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
