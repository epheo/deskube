package services

import (
	"fmt"
	"log"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/types"
)

func InstallKubeApiServer(globalData types.GlobalData) {

	// Define the certificate to generate
	certData := types.CertData{
		CN:    "kubernetes",
		O:     "Kubernetes",
		Hosts: []string{"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster", fmt.Sprintf("kubernetes.svc.%s", globalData.ClusterDomain), "127.0.0.1", globalData.ClusterIp, globalData.IpAddress},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	_, _, err := certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

}