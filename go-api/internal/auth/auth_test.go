package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestJWTRoundTripAndTamper(t *testing.T) {
	j, err := NewJWT("", "", time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := j.Generate("admin", "ADMIN")
	u, r, err := j.Parse(tok)
	if err != nil || u != "admin" || r != "ADMIN" {
		t.Fatalf("got %q %q %v", u, r, err)
	}
	parts := strings.Split(tok, ".")
	forged := parts[0] + "." + b64.EncodeToString([]byte(`{"sub":"admin","role":"ADMIN","exp":9999999999}`)) + "." + parts[2]
	if _, _, err := j.Parse(forged); err == nil {
		t.Fatal("tampered payload accepted")
	}
	none := b64.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."
	if _, _, err := j.Parse(none); err == nil {
		t.Fatal("alg=none accepted")
	}
	if _, _, err := j.Parse("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestJWTExpiry(t *testing.T) {
	j, _ := NewJWT("", "", time.Hour, false)
	tok, _ := j.Generate("a", "EDITOR")
	j.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, _, err := j.Parse(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestJWTRequiresKeysInProd(t *testing.T) {
	if _, err := NewJWT("", "", time.Hour, true); err == nil {
		t.Fatal("prod without keys must refuse to start")
	}
}

func TestJWTLoadsSpringStyleKeysAndRejectsOtherKey(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	privDER, _ := x509.MarshalPKCS8PrivateKey(k)
	pubDER, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	j, err := NewJWT(base64.StdEncoding.EncodeToString(privDER), base64.StdEncoding.EncodeToString(pubDER), time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := j.Generate("x", "ADMIN")
	// a different key must not verify
	other, _ := NewJWT("", "", time.Hour, false)
	if _, _, err := other.Parse(tok); err == nil {
		t.Fatal("token from another key accepted")
	}
	// and a token signed by us verifies against the loaded public key via the stdlib directly
	parts := strings.Split(tok, ".")
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, _ := b64.DecodeString(parts[2])
	if rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA256, sum[:], sig) != nil {
		t.Fatal("signature not standard RS256")
	}
}

func TestPBKDF2(t *testing.T) {
	h := PBKDF2{Iterations: 1000}
	a, _ := h.Hash("secret123")
	b, _ := h.Hash("secret123")
	if a == b {
		t.Fatal("salt not random")
	}
	if !h.Verify("secret123", a) || h.Verify("wrong", a) || h.Verify("secret123", "junk") {
		t.Fatal("verify wrong")
	}
}
