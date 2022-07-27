package cmd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/spf13/cobra"
)

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

		fmt.Println(string(keyPEM), string(pubPEM))

		return nil
	},
}

func init() {
	keysCmd.AddCommand(keysGenerateCmd)
	rootCmd.AddCommand(keysCmd)
}
