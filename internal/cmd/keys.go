package cmd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func formatPemAsConstChar(data []byte) string {
	var b strings.Builder

	b.WriteString(`const char *kPublicRSAKeys[] = { "\`)
	b.WriteRune('\n')

	lines := strings.Split(string(data), "\n")
	var nonBlankLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonBlankLines = append(nonBlankLines, line)
	}

	lastLineIndex := len(nonBlankLines) - 1
	for i, line := range nonBlankLines {
		b.WriteString(strings.TrimSpace(line))
		switch i {
		case lastLineIndex:
			b.WriteString(`" };`)
		default:
			b.WriteString(`\n\`)
			b.WriteRune('\n')
		}
	}

	return b.String()
}

func newKeys(_ *application) *cobra.Command {
	var keysCmd = &cobra.Command{
		Use:   "keys",
		Short: "Keys management",
	}
	var keysGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate new RSA private key",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Generate RSA key.

			const bitSize = 2048
			key, err := rsa.GenerateKey(rand.Reader, bitSize)
			if err != nil {
				return nil
			}

			// Extract public component.
			pub := key.Public()

			// Encode private key to PKCS#1 ASN.1 PEM.
			keyPEM := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key),
				},
			)

			// Encode public key to PKCS#1 ASN.1 PEM.
			pubPEM := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PUBLIC KEY",
					Bytes: x509.MarshalPKCS1PublicKey(pub.(*rsa.PublicKey)),
				},
			)

			fmt.Println(string(keyPEM))
			fmt.Println(formatPemAsConstChar(pubPEM))

			return nil
		},
	}
	keysCmd.AddCommand(keysGenerateCmd)
	return keysCmd
}
