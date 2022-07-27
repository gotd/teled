// Package key implements RSA private key management.
package key

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/go-faster/errors"
)

// ParsePrivateKey parses PEM encoded private key.
func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block.Type != "RSA PRIVATE KEY" {
		return nil, errors.Errorf("unsupported key type: %q", block.Type)
	}

	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "PKCS1")
	}

	return k, nil
}
