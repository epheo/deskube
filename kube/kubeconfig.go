package kubeconfig

import (
	"fmt"
	"log"
	"os"

	"github.com/epheo/deskube/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func GenerateKubeconfig(globalData types.GlobalData, service types.Service, cert []byte, key []byte) {
	dir := "out/kubeconfig"
	kubeconfigPath := fmt.Sprintf("%s/%s.kubeconfig", dir, service.Name)

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
	config.AuthInfos[fmt.Sprintf("system:node:%s", service.Name)] = &api.AuthInfo{
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
