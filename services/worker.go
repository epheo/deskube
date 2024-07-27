package services

import (
	"log"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/kube"
	"github.com/epheo/deskube/types"
)

func InstallWorker(globalData types.GlobalData) {

	service := types.Service{
		Name:   "worker",
		User:   "system:node:worker",
		Server: globalData.IpAddress,
	}

	// Define the certificate to generate
	certData := types.CertData{
		CN:    "system:node:worker",
		O:     "system:nodes",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err := certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	kubeconfig.GenerateKubeconfig(globalData, service, cert, key)

}
