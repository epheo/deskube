package kubeconfig

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	// "os/exec"
)

func InstallKubectl() {
	url := "https://storage.googleapis.com/kubernetes-release/release"
	stableVersionURL := url + "/stable.txt"

	// Get stable version
	resp, err := http.Get(stableVersionURL)
	if err != nil {
		log.Println("Error fetching stable version:", err)
		return
	}
	defer resp.Body.Close()

	versionBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return
	}
	version := string(versionBytes)

	// Download kubectl
	kubectlURL := fmt.Sprintf("%s/%s/bin/linux/amd64/kubectl", url, version)
	kubectlResp, err := http.Get(kubectlURL)
	if err != nil {
		log.Println("Error downloading kubectl:", err)
		return
	}
	defer kubectlResp.Body.Close()

	out, err := os.Create("kubectl")
	if err != nil {
		log.Println("Error creating file:", err)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, kubectlResp.Body)
	if err != nil {
		log.Println("Error writing to file:", err)
		return
	}

	// Make kubectl executable
	if err := os.Chmod("kubectl", 0755); err != nil {
		log.Println("Error changing file permissions:", err)
		return
	}

	// Move kubectl to /usr/local/bin
	// cmd := exec.Command("sudo", "install", "kubectl", "/usr/local/bin/")
	// if err := cmd.Run(); err != nil {
	// 	fmt.Println("Error installing kubectl:", err)
	// 	return
	// }

	log.Println("kubectl installed successfully")
}
