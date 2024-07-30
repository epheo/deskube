package nodes

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/containerd/containerd/pkg/cri/config"
	"github.com/epheo/deskube/internal/github"
	"github.com/epheo/deskube/internal/googlestorage"
	"github.com/epheo/deskube/internal/system"
	"github.com/epheo/deskube/types"
	"github.com/pelletier/go-toml"
)

func Worker(globalData types.GlobalData) {
	log.Println("Worker node")

	packages := []string{
		"socat",
		"conntrack",
		"ipset",
		"iptables",
		"systemd-resolved",
		"container-selinux",
	}

	system.InstallSysPkg(packages)

	// Deactivate swap
	cmd := exec.Command("swapoff", "-a")
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to execute command: %v", err)
	}
	log.Println("Successfully deactivated swap")

	// Remove zram-generator-defaults
	system.RemoveSysPkg([]string{"zram-generator-defaults"})

	// Load kernel modules
	modules := []string{
		"nf_conntrack",
		"br_netfilter",
	}
	for _, module := range modules {
		cmd := exec.Command("modprobe", module)
		err := cmd.Run()
		if err != nil {
			log.Fatalf("Failed to execute command: %v", err)
		}
	}

	// Load kernel modules on boot
	// TODO

	// Enable and restart systemd-resolved
	system.EnableStartService([]string{"/usr/lib/systemd/system/systemd-resolved.service"})

	// Create directories
	directories := []string{
		"/etc/cni/net.d",
		"/opt/cni/bin",
		"/var/lib/kubelet",
		"/var/lib/kube-proxy",
		"/var/lib/kubernetes",
		"/var/run/kubernetes",
	}
	for _, dir := range directories {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kubectl")
	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kube-proxy")
	googlestorage.GoogleStorageDownload("kubernetes-release", "linux/amd64", "kubelet")

	binaries := []string{
		"out/bin/kubectl",
		"out/bin/kube-proxy",
		"out/bin/kubelet",
	}

	system.InstallBin(binaries, "/usr/local/bin")

	// kubernetes-sigs/cri-tools

	sourceDir := github.GithubDownload("kubernetes-sigs/cri-tools", "linux-amd64", "out/bin")
	log.Printf("Source dir: %s\n", sourceDir)

	foundFiles, err := system.FindFiles(sourceDir, []string{"crictl"})
	if err != nil {
		log.Println("Error:", err)
		return
	}
	system.InstallBin(foundFiles, "/usr/local/bin")

	// opencontainers/runc
	sourceDir = github.GithubDownload("opencontainers/runc", "amd64", "out/bin")
	log.Printf("Source dir: %s\n", sourceDir)
	system.MoveFile("out/bin/runc.amd64", "out/bin/runc")
	system.InstallBin([]string{"out/bin/runc"}, "/usr/local/bin")

	// containernetworking/plugins
	sourceDir = github.GithubDownload("containernetworking/plugins", "linux-amd64", "/opt/cni/bin")
	log.Printf("Source dir: %s\n", sourceDir)

	// containerd/containerd
	sourceDir = github.GithubDownload("containerd/containerd", "linux-amd64", "out/bin/containerd")
	log.Printf("Source dir: %s\n", sourceDir)

	targets := []string{
		"containerd",
		"containerd-shim",
		"containerd-shim-runc-v1",
		"containerd-shim-runc-v2",
		"containerd-stress",
		"ctr",
	}

	foundFiles, err = system.FindFiles("out/bin/containerd/bin/", targets)
	if err != nil {
		log.Println("Error:", err)
		return
	}
	system.InstallBin(foundFiles, "/bin")

	system.TemplateFile(
		"templates/worker/10-bridge.conf.tmpl",
		"/etc/cni/net.d/10-bridge.conf",
		globalData,
	)
	system.TemplateFile(
		"templates/worker/99-loopback.conf.tmpl",
		"/etc/cni/net.d/99-loopback.conf",
		globalData,
	)

	if err := os.MkdirAll("/etc/containerd/", 0755); err != nil {
		log.Fatal(err)
	}

	config := config.DefaultConfig()
	config.SystemdCgroup = true
	configFile := "/etc/containerd/config.toml"

	// Create or open the config file
	file, err := os.OpenFile(configFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Error opening config file: %v", err)
	}
	defer file.Close()

	// Write the default configuration to the file
	if err := toml.NewEncoder(file).Encode(config); err != nil {
		log.Fatalf("Error writing config file: %v", err)
	}

	log.Println("Default containerd configuration written to", configFile)

	system.TemplateFile(
		"templates/worker/containerd.service.tmpl",
		"/etc/systemd/system/containerd.service",
		globalData,
	)

	system.CopyFile(
		"out/pem/ca.crt",
		"/var/lib/kubernetes/ca.crt",
	)
	system.CopyFile(
		fmt.Sprintf("out/pem/%s.crt", globalData.WorkerHostname),
		fmt.Sprintf("/var/lib/kubelet/%s.crt", globalData.WorkerHostname),
	)
	system.CopyFile(
		fmt.Sprintf("out/pem/%s.key", globalData.WorkerHostname),
		fmt.Sprintf("/var/lib/kubelet/%s.key", globalData.WorkerHostname),
	)
	system.CopyFile(
		fmt.Sprintf("out/kubeconfig/%s.kubeconfig", globalData.WorkerHostname),
		"/var/lib/kubelet/kubeconfig",
	)

	system.TemplateFile(
		"templates/worker/kubelet-config.yaml.tmpl",
		"/var/lib/kubelet/kubelet-config.yaml",
		globalData,
	)
	system.TemplateFile(
		"templates/worker/kubelet.service.tmpl",
		"/etc/systemd/system/kubelet.service",
		globalData,
	)

	// kube-proxy

	system.CopyFile(
		"out/kubeconfig/kube-proxy.kubeconfig",
		"/var/lib/kube-proxy/kubeconfig",
	)
	system.TemplateFile(
		"templates/worker/kube-proxy-config.yaml.tmpl",
		"/var/lib/kube-proxy/kube-proxy-config.yaml",
		globalData,
	)
	system.TemplateFile(
		"templates/worker/kube-proxy.service.tmpl",
		"/etc/systemd/system/kube-proxy.service",
		globalData,
	)

	targets = []string{
		"/etc/systemd/system/containerd.service",
		"/etc/systemd/system/kubelet.service",
		"/etc/systemd/system/kube-proxy.service",
	}

	system.EnableStartService(targets)

}
