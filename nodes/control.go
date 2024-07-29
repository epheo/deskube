package nodes

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/epheo/deskube/internal/googlestorage"
	"github.com/epheo/deskube/internal/net"
	"github.com/epheo/deskube/internal/system"
	"github.com/epheo/deskube/types"
)

func Controller(globalData types.GlobalData) {
	log.Println("Downloading k8s binaries")

	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kube-apiserver")
	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kube-controller-manager")
	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kube-scheduler")
	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kubectl")

	binaries := []string{
		"out/bin/kube-apiserver",
		"out/bin/kube-controller-manager",
		"out/bin/kube-scheduler",
		"out/bin/kubectl",
	}

	system.InstallBin(binaries, "/usr/local/bin")

	if err := os.MkdirAll("/var/lib/kubernetes", 0755); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll("/etc/kubernetes/config", 0755); err != nil {
		log.Fatal(err)
	}

	filesToCopy := []string{
		"ca.crt",
		"ca.key",
		"kubernetes.crt",
		"kubernetes.key",
		"service-account.key",
		"service-account.crt",
	}

	for _, file := range filesToCopy {
		system.CopyFile(
			fmt.Sprintf("out/pem/%s", file),
			fmt.Sprintf("/var/lib/kubernetes/%s", file),
		)
	}

	system.CopyFile(
		"out/encryption-config.yaml",
		"/var/lib/kubernetes/encryption-config.yaml",
	)

	filesToCopy = []string{
		"kube-controller-manager.kubeconfig",
		"kube-scheduler.kubeconfig",
	}

	for _, file := range filesToCopy {
		system.CopyFile(
			fmt.Sprintf("out/kubeconfig/%s", file),
			fmt.Sprintf("/var/lib/kubernetes/%s", file),
		)
	}

	system.TemplateFile(
		"services/templates/kube-apiserver.service.tmpl",
		"/etc/systemd/system/kube-apiserver.service",
		globalData,
	)

	system.TemplateFile(
		"services/templates/kube-controller-manager.service.tmpl",
		"/etc/systemd/system/kube-controller-manager.service",
		globalData,
	)
	system.TemplateFile(
		"services/templates/kube-scheduler.yaml.tmpl",
		"/etc/kubernetes/config/kube-scheduler.yaml",
		globalData,
	)
	system.TemplateFile(
		"services/templates/kube-scheduler.service.tmpl",
		"/etc/systemd/system/kube-scheduler.service",
		globalData,
	)

	services := []string{
		"/etc/systemd/system/kube-apiserver.service",
		"/etc/systemd/system/kube-controller-manager.service",
		"/etc/systemd/system/kube-scheduler.service",
	}

	system.EnableStartService(services)

	system.InstallSysPkg([]string{"nginx"})

	system.TemplateFile(
		"services/templates/nginx.conf.tmpl",
		fmt.Sprintf("/etc/nginx/conf.d/kubernetes.default.svc.%s.conf", globalData.ClusterDomain),
		globalData,
	)

	// is this still necessary?
	cmd := exec.Command("setsebool", "httpd_can_network_connect", "1", "-P")
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to execute command: %v", err)
	}
	log.Println("Successfully set SELinux boolean httpd_can_network_connect to 1")

	system.EnableStartService([]string{"/usr/lib/systemd/system/nginx.service"})

	// Wait for the kube-api endpoint to become available
	net.WaitForEndpoint(
		"http://127.0.0.1/healthz",
		fmt.Sprintf("kubernetes.default.svc.%s", globalData.ClusterDomain),
		time.Minute,
		3*time.Second,
	)

}
