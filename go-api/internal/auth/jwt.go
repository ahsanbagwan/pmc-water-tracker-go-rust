// Package auth: RS256 JWT (stdlib only, token-compatible with the Spring jjwt tokens: claims
// sub, role, iat, exp) and password hashing.
package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type JWT struct {
	priv *rsa.PrivateKey
	pub  *rsa.PublicKey
	ttl  time.Duration
	now  func() time.Time
}

var ErrInvalidToken = errors.New("invalid token")

// NewJWT: base64 DER keys (PKCS8 private, X509 public) exactly as the Spring env vars hold them.
// With both empty a throwaway keypair is generated (dev only) unless requireKeys is set (prod).
func NewJWT(privB64, pubB64 string, ttl time.Duration, requireKeys bool) (*JWT, error) {
	j := &JWT{ttl: ttl, now: time.Now}
	if privB64 == "" || pubB64 == "" {
		if requireKeys {
			return nil, errors.New("refusing to start: prod profile but JWT_PRIVATE_KEY_B64/JWT_PUBLIC_KEY_B64 are unset")
		}
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		j.priv, j.pub = k, &k.PublicKey
		return j, nil
	}
	pd, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	pk, err := x509.ParsePKCS8PrivateKey(pd)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	rk, ok := pk.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	ud, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	pubAny, err := x509.ParsePKIXPublicKey(ud)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not RSA")
	}
	j.priv, j.pub = rk, pub
	return j, nil
}

var b64 = base64.RawURLEncoding

func (j *JWT) Generate(username, role string) (string, error) {
	now := j.now()
	header := b64.EncodeToString([]byte(`{"alg":"RS256"}`))
	claims, _ := json.Marshal(map[string]any{
		"sub": username, "role": role, "iat": now.Unix(), "exp": now.Add(j.ttl).Unix(),
	})
	signing := header + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, j.priv, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64.EncodeToString(sig), nil
}

// Parse verifies signature, algorithm and expiry and returns (username, role).
func (j *JWT) Parse(token string) (string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", ErrInvalidToken
	}
	hdrRaw, err := b64.DecodeString(parts[0])
	if err != nil {
		return "", "", ErrInvalidToken
	}
	var hdr struct{ Alg string }
	if json.Unmarshal(hdrRaw, &hdr) != nil || hdr.Alg != "RS256" { // rejects alg=none / HS256 confusion
		return "", "", ErrInvalidToken
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", "", ErrInvalidToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(j.pub, crypto.SHA256, sum[:], sig) != nil {
		return "", "", ErrInvalidToken
	}
	raw, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", "", ErrInvalidToken
	}
	var c struct {
		Sub  string `json:"sub"`
		Role string `json:"role"`
		Exp  int64  `json:"exp"`
	}
	if json.Unmarshal(raw, &c) != nil || c.Sub == "" || c.Exp == 0 || !j.now().Before(time.Unix(c.Exp, 0)) {
		return "", "", ErrInvalidToken
	}
	return c.Sub, c.Role, nil
}
