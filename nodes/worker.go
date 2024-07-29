package nodes

import (
	"log"
	"os"
	"os/exec"

	"github.com/epheo/deskube/internal/github"
	"github.com/epheo/deskube/internal/googlestorage"
	"github.com/epheo/deskube/internal/system"
)

//
// url=https://github.com/kubernetes-sigs/cri-tools
// version=$(github_latest ${url})
// github_download ${url} ${version} crictl-v${version}-linux-amd64.tar.gz
// tar -xvf crictl-v${version}-linux-amd64.tar.gz
//
// url=https://github.com/opencontainers/runc
// version=$(github_latest ${url})
// github_download ${url} ${version} runc.amd64
// mv runc.amd64 runc
//
// url=https://github.com/containernetworking/plugins
// version=$(github_latest ${url})
// github_download ${url} ${version} cni-plugins-linux-amd64-v${version}.tgz
// sudo tar -xvf cni-plugins-linux-amd64-v${version}.tgz -C /opt/cni/bin/
//
// url=https://github.com/containerd/containerd
// version=$(github_latest ${url})
// github_download ${url} ${version} containerd-${version}-linux-amd64.tar.gz
// mkdir -p containerd
// tar -xvf containerd-${version}-linux-amd64.tar.gz -C containerd
//
// # Install the worker binaries
//
// chmod +x crictl kubectl kube-proxy kubelet runc
// sudo install crictl kubectl kube-proxy kubelet runc /usr/local/bin/
// sudo install containerd/bin/* /bin/

func Worker() {
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
	system.EnableStartService([]string{"systemd-resolved"})

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

	// system.InstallBin([]string{"out/bin/containerd"}, "/bin")

}
