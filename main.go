package main

import (
	"log"
	"os"
	"strings"

	"github.com/epheo/deskube/certificates"
	"github.com/epheo/deskube/internal/net"
	"github.com/epheo/deskube/k8s"
	"github.com/epheo/deskube/nodes"
	"github.com/epheo/deskube/services"
	"github.com/epheo/deskube/types"
)

func main() {

	// Generate CA
	caCert, caKey, err := certificates.GenerateCA()
	if err != nil {
		log.Fatalf("Error generating CA: %v", err)
	}

	// Get the full worker hostname
	fullHostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Error getting hostname: %v", err)
	}
	// Extract the short hostname (part before the first dot)
	WorkerHostname := strings.Split(fullHostname, ".")[0]

	globalData := types.GlobalData{
		CaKey:          caKey,
		CaCert:         caCert,
		IpAddress:      net.GetIpAddress(),
		ClusterIp:      "10.32.0.1",
		ClusterDNS:     "10.32.0.10",
		ClusterName:    "deskube",
		ClusterDomain:  "cluster.local",
		ClusterNetwork: "10.200.0.0/16",
		Hostname:       "deskube",
		ServiceNetwork: "10.32.0.0/24",
		WorkerHostname: WorkerHostname,
	}

	services.InstallAdmin(globalData)

	services.InstallWorker(globalData)

	services.InstallKubeControllerManager(globalData)

	services.InstallKubeProxy(globalData)

	services.InstallKubeScheduler(globalData)

	services.InstallKubeApiServer(globalData)

	k8s.GenerateEncryptionConfig()

	services.InstallEtcd(globalData)

	nodes.Controller(globalData)

	nodes.Worker(globalData)

}
