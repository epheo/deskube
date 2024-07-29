package k8s

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/epheo/deskube/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func GenerateKubeconfig(globalData types.GlobalData, service types.Service, cert []byte, key []byte) {
	dir := "out/kubeconfig"

	tokens := strings.Split(service.User, ":")
	fileName := tokens[len(tokens)-1]
	kubeconfigPath := fmt.Sprintf("%s/%s.kubeconfig", dir, fileName)

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return
	}

	config := api.NewConfig()

	// Load CA certificate
	caCert, err := os.ReadFile("out/pem/ca.crt")
	if err != nil {
		return
	}

	// Set Cluster
	config.Clusters[globalData.ClusterName] = &api.Cluster{
		Server:                   fmt.Sprintf("https://%s:6443", globalData.IpAddress),
		CertificateAuthorityData: []byte(caCert),
	}

	// Set Credentials
	config.AuthInfos[service.User] = &api.AuthInfo{
		ClientCertificateData: []byte(cert),
		ClientKeyData:         []byte(key),
	}

	// Set Context
	config.Contexts["default"] = &api.Context{
		Cluster:  globalData.ClusterName,
		AuthInfo: service.User,
	}

	// Use Context
	config.CurrentContext = "default"

	// Save kubeconfig
	if err := clientcmd.WriteToFile(*config, kubeconfigPath); err != nil {
		log.Printf("Failed to write kubeconfig: %v\n", err)
		return
	}

	log.Println("Kubeconfig set up successfully")
}
