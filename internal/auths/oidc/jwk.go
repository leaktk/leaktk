package oidc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

type JWK struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (k *JWK) String() string {
	return fmt.Sprintf(
		"kty=%q alg=%q use=%q kid=%q n=%q e=%q",
		k.Kty, k.Alg, k.Use, k.Kid, k.N, k.E,
	)
}
func (k *JWK) LoadRSAPubKey(pubKey *rsa.PublicKey) error {
	var e int
	var n big.Int
	var value []byte
	var err error

	if k.Kty != "RSA" {
		return fmt.Errorf("unsupported key type: expected=\"RSA\" actual=%q", k.Kty)
	}

	value, err = base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return fmt.Errorf("failed to decode modulus: %w", err)
	}
	n.SetBytes(value)

	value, err = base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return fmt.Errorf("failed to decode exponent: %w", err)
	}
	for _, b := range value {
		e = (e << 8) | int(b)
	}
	*pubKey = rsa.PublicKey{N: &n, E: e}
	return nil
}

func (k *JWK) VerifyRS256(jwt []byte) error {
	i := bytes.LastIndexByte(jwt, '.')
	if i < 0 || i+1 >= len(jwt) {
		return errors.New("invalid JWK format")
	}
	headAndBodyB64 := jwt[:i]
	sigB64 := jwt[i+1:]

	var pubKey rsa.PublicKey
	if err := k.LoadRSAPubKey(&pubKey); err != nil {
		return fmt.Errorf("failed to load RSA public key: %w %s", err, k.String())
	}

	sig := make([]byte, base64.RawURLEncoding.DecodedLen(len(sigB64)))
	if _, err := base64.RawURLEncoding.Decode(sig, sigB64); err != nil {
		return fmt.Errorf("failed to decode JWT signature: %w %s", err, k.String())
	}

	hashed := sha256.Sum256(jwt[:len(headAndBodyB64)])
	if err := rsa.VerifyPKCS1v15(&pubKey, crypto.SHA256, hashed[:], sig); err != nil {
		return fmt.Errorf("signature verification failed: %w %s", err, k.String())
	}

	return nil
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

func FetchJWKs(ctx context.Context, jwks *JWKS, jwksURL string) error {
	logger.Debug("fetching JWKs: url=%q", jwksURL)
	client := httpclient.NewClient()
	resp, err := client.Get(jwksURL)
	if err != nil {
		return fmt.Errorf("fetch jwks: %v url=%q", err, jwksURL)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: unexpected status code: status_code=%d url=%q", resp.StatusCode, jwksURL)
	}

	d := json.NewDecoder(resp.Body)
	err = d.Decode(jwks)
	if err != nil {
		return fmt.Errorf("decode jwks resp: %v url=%q", err, jwksURL)
	}
	return nil
}
