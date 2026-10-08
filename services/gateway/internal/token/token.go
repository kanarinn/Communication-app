package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Claims struct {
	Sub      string `json:"sub"` // user id
	Username string `json:"username"`
	Exp      int64  `json:"exp"` // expiry, unix seconds
}

var (
	ErrMalformed = errors.New("token: malformed")
	ErrSignature = errors.New("token: bad signature")
	ErrExpired   = errors.New("token: expired")
)

var enc = base64.RawURLEncoding

const header = `{"alg":"HS256","typ":"JWT"}`

func Sign(secret []byte, c Claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signingInput := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString(payload)
	return signingInput + "." + enc.EncodeToString(mac(secret, signingInput)), nil
}

func Verify(secret []byte, tok string) (Claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}
	hdr, err := enc.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(hdr, &h); err != nil || h.Alg != "HS256" {
		return Claims{}, ErrMalformed
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	if !hmac.Equal(sig, mac(secret, parts[0]+"."+parts[1])) {
		return Claims{}, ErrSignature
	}
	payload, err := enc.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrMalformed
	}
	if c.Exp != 0 && time.Now().Unix() >= c.Exp {
		return Claims{}, ErrExpired
	}
	return c, nil
}

func mac(secret []byte, input string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(input))
	return m.Sum(nil)
}
