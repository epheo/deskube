package system

import (
	"fmt"
	"log"
	"os/exec"
)

func InstallSysPkg(pkgNames []string) {
	// Define the dnf command and arguments
	cmd := exec.Command("sudo", append([]string{"dnf", "install", "-y"}, pkgNames...)...)

	// Execute the command and capture the output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to execute command: %v", err)
	}

	// Print the output of the command
	fmt.Printf("Command output:\n%s\n", string(output))
}
