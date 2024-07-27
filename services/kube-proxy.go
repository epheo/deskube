package services

import (
	"log"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/kube"
	"github.com/epheo/deskube/types"
)

func InstallKubeProxy(globalData types.GlobalData) {

	service := types.Service{
		Name:   "kube-proxy",
		User:   "system:kube-proxy",
		Server: globalData.IpAddress,
	}

	// Define the certificate to generate
	certData := types.CertData{
		CN:    "system:kube-proxy",
		O:     "system:node-proxier",
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