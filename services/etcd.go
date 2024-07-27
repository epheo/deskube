package services

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/epheo/deskube/internal/github"
	"github.com/epheo/deskube/internal/system"
	"github.com/epheo/deskube/types"
	"github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

func InstallEtcd(globalData types.GlobalData) {

	sourceDir := github.GithubDownload("etcd-io/etcd", "linux-amd64")
	destinationDir := "/usr/local/bin"
	log.Printf("Source dir: %s\n", sourceDir)

	targets := []string{"etcd", "etcdctl", "etcdutl"}

	foundFiles, err := system.FindFiles(sourceDir, targets)
	if err != nil {
		log.Println("Error:", err)
		return
	}

	system.InstallBin(foundFiles, destinationDir)

	// Restore SELinux context

	files := []string{
		"/usr/local/bin/etcd",
		"/usr/local/bin/etcdctl",
		"/usr/local/bin/etcdutl",
	}

	for _, file := range files {
		if !selinux.GetEnabled() {
			log.Printf("SELinux is not enabled on this system")
		}

		err := label.Relabel(file, "", true)
		if err != nil {
			log.Printf("Failed to restore SELinux context: %v", err)
		} else {
			log.Printf("Successfully restored SELinux context for %s\n", file)
		}
	}

	// Create directories
	err = os.MkdirAll("/etc/etcd", 0755)
	if err != nil {
		log.Printf("Failed to create directory /etc/etcd: %v\n", err)
		return
	}

	err = os.MkdirAll("/var/lib/etcd", 0700)
	if err != nil {
		log.Printf("Failed to create directory /var/lib/etcd: %v\n", err)
		return
	}

	certs := []string{
		"ca.crt",
		"kubernetes.key",
		"kubernetes.crt",
	}
	for _, file := range certs {
		dir := "out/pem/"
		src := dir + file
		dst := filepath.Join("/etc/etcd", file)
		err = system.CopyFile(src, dst)
		if err != nil {
			log.Printf("Failed to copy %s to %s: %v\n", src, dst, err)
			return
		}
	}

	system.TemplateFile(
		"services/templates/etcd.service.tmpl",
		"/etc/systemd/system/etcd.service",
		globalData,
	)
	system.EnableStartService([]string{"/etc/systemd/system/etcd.service"})

	cmd := exec.Command(
		"sudo", "ETCDCTL_API=3", "/usr/local/bin/etcdctl", "member", "list",
		"--endpoints=https://"+globalData.IpAddress+":2379",
		"--cacert=/etc/etcd/ca.crt",
		"--cert=/etc/etcd/kubernetes.crt",
		"--key=/etc/etcd/kubernetes.key")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	log.Printf("Etcd output:\n%s\n", string(output))

}
