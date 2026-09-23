package xray

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/curve25519"
)

// RealityKeys holds a Reality server keypair plus a short-id.
type RealityKeys struct {
	Priv string `json:"priv"`
	Pub  string `json:"pub"`
	SID  string `json:"sid"`
}

// GenerateReality creates a fresh X25519 keypair (Xray urlsafe-base64, no
// padding) and a random 8-hex short-id. Keys are persisted in Postgres so
// they survive every redeploy.
func GenerateReality() (RealityKeys, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return RealityKeys{}, err
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return RealityKeys{}, err
	}
	sid := make([]byte, 4)
	if _, err := rand.Read(sid); err != nil {
		return RealityKeys{}, err
	}
	enc := base64.RawURLEncoding
	return RealityKeys{
		Priv: enc.EncodeToString(priv),
		Pub:  enc.EncodeToString(pub),
		SID:  hex.EncodeToString(sid),
	}, nil
}
