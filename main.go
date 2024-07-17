package main

import (
	"fmt"
	"log"

	"github.com/cloudflare/cfssl/config"
	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/internal/github"
	"github.com/epheo/deskube/internal/net"
	"github.com/epheo/deskube/kube"
	"github.com/epheo/deskube/types"
)

func main() {

	clusterIp := "10.32.0.1"
	clusterName := "deskube"
	clusterDomain := "cluster.local"

	// Generate CA
	caCert, caKey, err := certificates.GenerateCA()
	if err != nil {
		log.Fatalf("Error generating CA: %v", err)
	}

	globalData := types.GlobalData{
		CaKey:         caKey,
		CaCert:        caCert,
		IpAddress:     net.GetIpAddress(),
		ClusterIp:     clusterIp,
		ClusterName:   clusterName,
		ClusterDomain: clusterDomain,
	}

	//////////////////////////////////////////
	// admin
	//////////////////////////////////////////

	service := types.Service{
		Name:   "admin",
		User:   "admin",
		Server: "127.0.0.1",
	}

	// Define the certificate to generate
	certData := types.CertData{
		CN:    "admin",
		O:     "system:masters",
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

	//////////////////////////////////////////
	// worker
	//////////////////////////////////////////

	service = types.Service{
		Name:   "worker",
		User:   "system:node:worker",
		Server: globalData.IpAddress,
	}

	// Define the certificate to generate
	certData = types.CertData{
		CN:    "system:node:worker",
		O:     "system:nodes",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err = certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	kubeconfig.GenerateKubeconfig(globalData, service, cert, key)

	//////////////////////////////////////////
	// kube-controller-manager
	//////////////////////////////////////////

	service = types.Service{
		Name:   "kube-controller-manager",
		User:   "system:kube-controller-manager",
		Server: "127.0.0.1",
	}

	// Define the certificate to generate
	certData = types.CertData{
		CN:    "system:kube-controller-manager",
		O:     "system:kube-controller-manager",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err = certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	kubeconfig.GenerateKubeconfig(globalData, service, cert, key)

	//////////////////////////////////////////
	// proxy
	//////////////////////////////////////////

	service = types.Service{
		Name:   "kube-proxy",
		User:   "system:kube-proxy",
		Server: globalData.IpAddress,
	}

	// Define the certificate to generate
	certData = types.CertData{
		CN:    "system:kube-proxy",
		O:     "system:node-proxier",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err = certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	kubeconfig.GenerateKubeconfig(globalData, service, cert, key)

	//////////////////////////////////////////
	// kube-scheduler
	//////////////////////////////////////////

	service = types.Service{
		Name:   "kube-scheduler",
		User:   "system:kube-scheduler",
		Server: "127.0.0.1",
	}

	// Define the certificate to generate
	certData = types.CertData{
		CN:    "system:kube-scheduler",
		O:     "system:kube-scheduler",
		Hosts: []string{""},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	cert, key, err = certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	kubeconfig.GenerateKubeconfig(globalData, service, cert, key)

	//////////////////////////////////////////
	// kubernetes API server
	//////////////////////////////////////////

	// Define the certificate to generate
	certData = types.CertData{
		CN:    "kubernetes",
		O:     "Kubernetes",
		Hosts: []string{"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster", fmt.Sprintf("kubernetes.svc.%s", globalData.ClusterDomain), "127.0.0.1", globalData.ClusterIp, globalData.IpAddress},
		Config: &config.SigningProfile{
			Usage:        []string{"server auth"},
			Expiry:       8760,
			CAConstraint: config.CAConstraint{IsCA: false},
		},
	}
	_, _, err = certificates.GenerateCert(certData, globalData)
	if err != nil {
		log.Fatalf("Error generating %s certificate: %v", certData.CN, err)
	}

	//////////////////////////////////////////

	kubeconfig.GenerateEncryptionConfig()
	github.GithubDownload("etcd-io/etcd")

}
