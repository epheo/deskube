package main

import (
	"log"

	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/internal/net"
	"github.com/epheo/deskube/kube"
	"github.com/epheo/deskube/services"
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
		Hostname:      "deskube",
	}

	services.InstallAdmin(globalData)

	services.InstallWorker(globalData)

	services.InstallKubeControllerManager(globalData)

	services.InstallKubeProxy(globalData)

	services.InstallKubeScheduler(globalData)

	services.InstallKubeApiServer(globalData)

	kubeconfig.GenerateEncryptionConfig()

	services.InstallEtcd(globalData)

}
