package id

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"github.com/cespare/xxhash/v2"

	"github.com/leaktk/leaktk/internal/logger"
)

// ID returns an unique ID based on the parts passed in. If none are passed in
// then ID will be random. The ID will be a fixed length characters.
func ID(parts ...string) string {
	data := make([]byte, 8)

	if len(parts) > 0 {
		hash := xxhash.Sum64String(strings.Join(parts, "\n"))
		binary.BigEndian.PutUint64(data, hash)
	} else {
		if _, err := rand.Read(data); err != nil {
			logger.Fatal("could not generate random id: error=%q", err)
		}
	}

	return base64.RawURLEncoding.EncodeToString(data)
}
