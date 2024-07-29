package services

import (
	"log"
	"time"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/k8s"
	"github.com/epheo/deskube/types"
)

func InstallKubeProxy(globalData types.GlobalData) {

	service := types.Service{
		User:   "system:kube-proxy",
		Server: globalData.IpAddress,
	}

	// Define the certificate to generate
	certData := types.CertData{
		CN:    "system:kube-proxy",
		O:     "system:node-proxier",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"signing", "key encipherment", "server auth", "client auth"},
			Expiry:       time.Hour * 24 * 365 * 10,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err := certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	k8s.GenerateKubeconfig(globalData, service, cert, key)

}
