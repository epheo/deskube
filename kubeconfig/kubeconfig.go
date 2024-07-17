package kubeconfig

import (
	"fmt"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func GenerateKubeconfig() {
	// Example for setting up the Kubernetes Worker Node kubeconfig
	instance := "worker_hostname"      // Replace with actual hostname
	clusterName := "your_cluster_name" // Replace with your cluster name
	ipAddress := "your_cluster_ip"     // Replace with your cluster IP address
	kubeconfigPath := fmt.Sprintf("%s.kubeconfig", instance)

	config := api.NewConfig()

	// Set Cluster
	config.Clusters[clusterName] = &api.Cluster{
		Server:                   fmt.Sprintf("https://%s:6443", ipAddress),
		CertificateAuthorityData: []byte("ca.pem"), // Replace with actual CA data
	}

	// Set Credentials
	config.AuthInfos[fmt.Sprintf("system:node:%s", instance)] = &api.AuthInfo{
		ClientCertificateData: []byte(fmt.Sprintf("%s.pem", instance)),     // Replace with actual certificate data
		ClientKeyData:         []byte(fmt.Sprintf("%s-key.pem", instance)), // Replace with actual key data
	}

	// Set Context
	config.Contexts["default"] = &api.Context{
		Cluster:  clusterName,
		AuthInfo: fmt.Sprintf("system:node:%s", instance),
	}

	// Use Context
	config.CurrentContext = "default"

	// Save kubeconfig
	if err := clientcmd.WriteToFile(*config, kubeconfigPath); err != nil {
		fmt.Printf("Failed to write kubeconfig: %v\n", err)
		return
	}

	fmt.Println("Kubeconfig set up successfully")
}
