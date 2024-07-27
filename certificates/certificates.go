package certificates

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	"github.com/epheo/deskube/types"
)

func GenerateCA() ([]byte, []byte, error) {

	// Define the directory where you want to save the certificate
	dir := "out/pem"

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, nil, err
	}

	// Ensure the directory exists
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, nil, err
	}

	// Check if CA files exist
	if _, err := os.Stat("out/pem/ca.crt"); err == nil {
		// CA files exist, load and return them
		caCert, err := os.ReadFile("out/pem/ca.crt")
		if err != nil {
			return nil, nil, err
		}
		caKey, err := os.ReadFile("out/pem/ca.key")
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
			Expiry: "87600h",
		},
	}
	caCert, _, caKey, err := initca.New(req)
	if err != nil {
		return nil, nil, err
	}

	// Save CA files
	err = saveToFile("out/pem/ca.crt", caCert)
	if err != nil {
		return nil, nil, err
	}
	err = saveToFile("out/pem/ca.key", caKey)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

// func GenerateCert(cn string, hosts []string, caCertPEM, caKeyPEM []byte, conf *config.SigningProfile) (cert []byte, key []byte, err error) {
func GenerateCert(certData types.CertData, globalData types.GlobalData) (cert []byte, key []byte, err error) {

	dir := "out/pem"

	// Parse the CA certificate
	caCertBlock, _ := pem.Decode(globalData.CaCert)
	if caCertBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// Parse the CA private key
	caKeyBlock, _ := pem.Decode(globalData.CaKey)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode ca.key")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse CA private key: %v", err)
	}

	// Generate CSR
	csrConfig := &csr.CertificateRequest{
		CN: certData.CN,
		Names: []csr.Name{
			{
				C:  "AQ",
				L:  "Antartica",
				O:  certData.O,
				OU: "Kubernetes",
				ST: "South Pole",
			},
		},
		Hosts:      certData.Hosts,
		KeyRequest: csr.NewKeyRequest(),
	}
	generatedCSR, generatedKey, err := csr.ParseRequest(csrConfig)
	if err != nil {
		return nil, nil, err
	}

	// Prepare CA signer
	SigningConfig := &config.Signing{Default: certData.Config}

	caSigner, err := local.NewSigner(caKey, caCert, signer.DefaultSigAlgo(caKey), SigningConfig)
	if err != nil {
		return nil, nil, err
	}

	// Sign the certificate
	signedCert, err := caSigner.Sign(signer.SignRequest{
		Request: string(generatedCSR),
		Profile: "kubernetes", // Specify the signing profile if needed
	})
	if err != nil {
		return nil, nil, err
	}

	// Save the signed certificate and private key to files

	tokens := strings.Split(certData.CN, ":")

	// Save the signed certificate to the specified directory
	certPath := dir + "/" + tokens[len(tokens)-1] + ".crt"
	err = os.WriteFile(certPath, signedCert, 0644)
	if err != nil {
		return nil, nil, err
	}

	keyPath := dir + "/" + tokens[len(tokens)-1] + ".key"
	err = os.WriteFile(keyPath, generatedKey, 0600)
	if err != nil {
		return nil, nil, err
	}

	return signedCert, generatedKey, nil
}

func saveToFile(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0644)
}
