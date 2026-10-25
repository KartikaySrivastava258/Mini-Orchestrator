package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUID returns 128 random bits as 32 hex chars. Names get reused, UIDs never do.
func NewUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// basically unreachable on real platforms, but if it happens we want to hear about it loudly
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// suffixAlphabet has no vowels, so generated names can't accidentally spell
// anything embarrassing. Kubernetes uses the same trick.
const suffixAlphabet = "bcdfghjklmnpqrstvwxz2456789"

// RandomSuffix returns n random chars for generated pod names like web-x7k2p.
func RandomSuffix(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	for i := range b {
		b[i] = suffixAlphabet[int(b[i])%len(suffixAlphabet)]
	}
	return string(b)
}
