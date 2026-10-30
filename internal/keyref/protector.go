// Package keyref converts opaque license keys into safe persistence references.
package keyref

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	// AlgorithmHMACSHA256 identifies the protected reference format.
	AlgorithmHMACSHA256 = "hmac-sha256"
	// minimumSecretLength prevents accidentally configuring a trivial protector.
	minimumSecretLength = 16
)

// Protector creates non-reversible references for opaque license keys.
type Protector struct {
	secret []byte
}

// NewProtector creates an HMAC-SHA256 protector from a configured secret.
func NewProtector(secret string) (*Protector, error) {
	if strings.TrimSpace(secret) != secret || len(secret) < minimumSecretLength {
		return nil, errors.New("license key protection secret is too short or not trimmed")
	}
	return &Protector{secret: []byte(secret)}, nil
}

// Protect returns a stable non-reversible reference for one raw license key.
// The input is not retained after the method returns.
func (protector *Protector) Protect(rawKey string) (string, error) {
	if protector == nil || len(protector.secret) == 0 {
		return "", errors.New("license key protector is required")
	}
	if strings.TrimSpace(rawKey) == "" || strings.TrimSpace(rawKey) != rawKey {
		return "", errors.New("license key is required and trimmed")
	}
	mac := hmac.New(sha256.New, protector.secret)
	_, _ = mac.Write([]byte(rawKey))
	return AlgorithmHMACSHA256 + ":" + hex.EncodeToString(mac.Sum(nil)), nil
}
