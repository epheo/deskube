package main

import (
	"log"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/kubeconfig"
)

func main() {
	// Generate CA
	caCert, caKey, err := certificates.GenerateCA()
	if err != nil {
		log.Fatalf("Error generating CA: %v", err)
	}

	// Define all the certificates to generate
	certs := []struct {
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

	for _, cert := range certs {
		err := certificates.GenerateCert(cert.CN, cert.Hosts, caCert, caKey, cert.Config)
		if err != nil {
			log.Fatalf("Error generating %s certificate: %v", cert.CN, err)
		}
	}

	kubeconfig.InstallKubectl()
	kubeconfig.GenerateKubeconfig()

}
