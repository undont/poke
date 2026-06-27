// package id mints short random correlation ids for pokes.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a 128-bit random id as a hex string.
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
