package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

var ErrNotFound = errors.New("user not found")

type Client struct {
	base string
	http *http.Client
}

func New(baseURL string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Client) ByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := c.get(ctx, "/internal/users/by-username/"+url.PathEscape(username), &u)
	return u, err
}

func (c *Client) ByIDs(ctx context.Context, ids []string) (map[string]User, error) {
	out := map[string]User{}
	if len(ids) == 0 {
		return out, nil
	}
	var list []User
	if err := c.get(ctx, "/internal/users?ids="+url.QueryEscape(strings.Join(ids, ",")), &list); err != nil {
		return nil, err
	}
	for _, u := range list {
		out[u.ID] = u
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("auth service: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("auth service: unexpected status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
