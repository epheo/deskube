package kubeconfig

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func GenerateEncryptionConfig() {
	dir := "out"
	configFileName := "out/encryption-config.yaml"

	// Ensure the directory exists
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return
	}

	// Check if the file exists
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		fmt.Println("Encryption key not found, generating a new one")

		// Generate a new encryption key
		encryptionKey, err := generateEncryptionKey(32)
		if err != nil {
			fmt.Println("Error generating encryption key:", err)
			return
		}

		// Create the encryption-config.yaml content
		configContent := fmt.Sprintf(`kind: EncryptionConfig
apiVersion: v1
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: %s
      - identity: {}
`, encryptionKey)

		// Write the content to encryption-config.yaml
		err = os.WriteFile(configFileName, []byte(configContent), 0644)
		if err != nil {
			fmt.Println("Error writing encryption-config.yaml:", err)
			return
		}
	} else if err != nil {
		fmt.Println("Error checking encryption-config.yaml:", err)
		return
	} else {
		fmt.Println("encryption-config.yaml already exists")
	}
}

// generateEncryptionKey generates a random encryption key of the specified length
func generateEncryptionKey(length int) (string, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}
