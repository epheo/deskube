package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
)

func main() {
	// Generate CA
	caCert, caKey, err := generateCA()
	if err != nil {
		log.Fatalf("Error generating CA: %v", err)
	}

	// Define all the certificates to generate
	certificates := []struct {
		CN     string
		Hosts  []string
		Config *config.SigningProfile
	}{
		//{"admin", nil, nil},
		{"web", []string{"example.com"}, &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		}},
		{"app", []string{"app.example.com"}, &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		}},
	}

	for _, cert := range certificates {
		err := generateCert(cert.CN, cert.Hosts, caCert, caKey, cert.Config)
		if err != nil {
			log.Fatalf("Error generating %s certificate: %v", cert.CN, err)
		}
	}
}

func generateCA() ([]byte, []byte, error) {
	// Check if CA files exist
	if _, err := os.Stat("ca.crt"); err == nil {
		// CA files exist, load and return them
		caCert, err := os.ReadFile("ca.crt")
		if err != nil {
			return nil, nil, err
		}
		caKey, err := os.ReadFile("ca.key")
		if err != nil {
			return nil, nil, err
		}
		return caCert, caKey, nil
	}

	// Generate new CA
	req := &csr.CertificateRequest{
		CN:         "Kubernetes",
		KeyRequest: csr.NewKeyRequest(),
		CA: &csr.CAConfig{
			Expiry: "8760h",
		},
	}
	caCert, _, caKey, err := initca.New(req)
	if err != nil {
		return nil, nil, err
	}

	// Save CA files
	err = saveToFile("ca.crt", caCert)
	if err != nil {
		return nil, nil, err
	}
	err = saveToFile("ca.key", caKey)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

func generateCert(cn string, hosts []string, caCertPEM, caKeyPEM []byte, conf *config.SigningProfile) error {

	// Parse the CA certificate
	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return fmt.Errorf("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// Parse the CA private key
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return fmt.Errorf("failed to decode CA key")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse CA private key: %v", err)
	}

	// Generate CSR
	csrConfig := &csr.CertificateRequest{
		CN: cn,
		Names: []csr.Name{
			{
				C:  "AQ",
				L:  "Antartica",
				O:  "system:masters",
				OU: "Kubernetes",
				ST: "South Pole",
			},
		},
		Hosts:      hosts,
		KeyRequest: csr.NewKeyRequest(),
	}
	generatedCSR, generatedKey, err := csr.ParseRequest(csrConfig)
	if err != nil {
		return err
	}

	// Prepare CA signer
	SigningConfig := &config.Signing{Default: conf}

	caSigner, err := local.NewSigner(caKey, caCert, signer.DefaultSigAlgo(caKey), SigningConfig)
	if err != nil {
		return err
	}

	// Sign the certificate
	signedCert, err := caSigner.Sign(signer.SignRequest{
		Request: string(generatedCSR),
		Profile: "kubernetes", // Specify the signing profile if needed
	})
	if err != nil {
		return err
	}

	// Save the signed certificate and private key to files
	// Define the directory where you want to save the certificate
	dir := "certificates"

	// Ensure the directory exists
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	// Save the signed certificate to the specified directory
	certPath := dir + "/" + cn + "-cert.pem"
	err = os.WriteFile(certPath, signedCert, 0644)
	if err != nil {
		return err
	}

	keyPath := dir + "/" + cn + "-key.pem"
	err = os.WriteFile(keyPath, generatedKey, 0600)
	if err != nil {
		return err
	}

	return nil
}

func saveToFile(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0644)
}
