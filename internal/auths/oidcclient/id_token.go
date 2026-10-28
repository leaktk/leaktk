package oidcclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

type IDToken struct {
	Value  []byte
	Header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	Claims struct {
		Iss      string `json:"iss"`
		Sub      string `json:"sub"`
		Aud      string `json:"aud"`
		Exp      int64  `json:"exp"`
		Iat      int64  `json:"iat"`
		AuthTime int64  `json:"auth_time"`
		Nonce    string `json:"nonce"`
		Acr      string `json:"acr"`
		Amr      string `json:"amr"`
		Azp      string `json:"azp"`
		Name     string `json:"name"`
		Jti      string `json:"jti"`
		AtHash   string `json:"at_hash"`
	}
}

func (t *IDToken) String() string {
	return string(t.Value)
}

func (t *IDToken) MarshalText() ([]byte, error) {
	return t.Value, nil
}

func (t *IDToken) UnmarshalText(text []byte) error {
	t.Value = text

	headerB64, bodyAndSigB64, found := bytes.Cut(text, []byte("."))
	if !found || len(headerB64) == 0 {
		return errors.New("jwt unmarshal: missing header")
	}

	header := make([]byte, b64u.DecodedLen(len(headerB64)))
	if _, err := b64u.Decode(header, headerB64); err != nil {
		return fmt.Errorf("jwt unmarshal: header: %w", err)
	}

	if err := json.Unmarshal(header, &t.Header); err != nil {
		return fmt.Errorf("jwt unmarshal: header: %w", err)
	}

	bodyB64, sigB64, found := bytes.Cut(bodyAndSigB64, []byte("."))
	if !found || len(bodyB64) == 0 {
		return errors.New("jwt unmarshal: missing body")
	} else if len(sigB64) == 0 {
		return errors.New("jwt unmarshal: missing signature")
	}
	body := make([]byte, b64u.DecodedLen(len(bodyB64)))
	if _, err := b64u.Decode(body, bodyB64); err != nil {
		return fmt.Errorf("jwt unmarshal: body: %w", err)
	}
	if err := json.Unmarshal(body, &t.Claims); err != nil {
		return fmt.Errorf("jwt unmarshal: body: %w", err)
	}
	return nil
}

func (t *IDToken) VerifyAccessToken(at string) error {
	if t.Header.Alg != "RS256" {
		return errors.New("this client can only verify RS256 access tokens")
	}

	atHash := []byte(t.Claims.AtHash)
	if len(atHash) == 0 {
		return errors.New("the id_token missing at_hash claim")
	}

	b := []byte(at)
	h := sha256.Sum256(b)
	d := make([]byte, b64u.EncodedLen(sha256.Size/2))
	b64u.Encode(d, h[:sha256.Size/2])
	if !bytes.Equal(atHash, d) {
		return fmt.Errorf("invalid at_hash: expected=%q actual=%q", atHash, d)
	}

	return nil
}
